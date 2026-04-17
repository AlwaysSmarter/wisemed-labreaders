package app

import (
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"wisemed-labreaders/new/internal/readeragent/analyzer"
	"wisemed-labreaders/new/internal/readeragent/comm"
	"wisemed-labreaders/new/internal/readeragent/command"
	"wisemed-labreaders/new/internal/readeragent/control"
	"wisemed-labreaders/new/internal/readeragent/storage"
	"wisemed-labreaders/new/internal/shared/config"
)

func Run(cfg *config.ReaderAgentConfig) error {
	adapter, err := analyzer.GetAdapter(cfg.Reader.AnalyzerCode)
	if err != nil {
		return err
	}

	store, err := storage.Open(cfg.Storage.LocalDBPath)
	if err != nil {
		return err
	}
	defer store.Close()

	h := &command.Handler{
		ReaderID:      cfg.Reader.ID,
		Adapter:       adapter,
		Store:         store,
		SetupComplete: strings.TrimSpace(cfg.Reader.MedicalUnitID) != "",
	}
	commMgr := comm.NewManager(store, adapter.Code())
	h.CommController = commMgr
	ws := &control.WSClient{
		WSURL:             cfg.Webservice.WSURL,
		APIBaseURL:        cfg.Webservice.APIBaseURL,
		ReconnectInterval: cfg.ReconnectInterval(),
		HeartbeatInterval: cfg.HeartbeatInterval(),
		ReaderID:          cfg.Reader.ID,
		AnalyzerCode:      cfg.Reader.AnalyzerCode,
		AnalyzerName:      cfg.Reader.AnalyzerName,
		AnalyzerType:      cfg.Reader.AnalyzerType,
		LicenseCode:       cfg.Reader.LicenseCode,
		APIKey:            cfg.Reader.APIKey,
		APIKeyRef:         cfg.Reader.APIKeyRef,
		Handler:           h,
		Store:             store,
	}
	commMgr.SetWorklistResolver(func(sampleID string, tags []string) ([]string, error) {
		return ws.ResolveWorklist(sampleID, "", tags)
	})

	if commCfg, err := store.GetCommunicationConfig(adapter.Code()); err == nil {
		if !h.SetupComplete {
			h.CommunicationStarted = false
			log.Printf("communication not started for %s: setup is not complete", adapter.Code())
		} else if err := commMgr.Start(commCfg); err != nil {
			h.CommunicationStarted = false
			log.Printf("communication failed to start for %s: %v", adapter.Code(), err)
		} else {
			h.CommunicationStarted = true
			log.Printf("communication started for %s via %s/%s", adapter.Code(), commCfg.Transport, commCfg.Mode)
		}
	} else {
		h.CommunicationStarted = false
		log.Printf("communication not started for %s: missing local communication config", adapter.Code())
	}
	ws.OnRegistrationState = func(setupComplete bool, profile map[string]interface{}) {
		if setupComplete == h.SetupComplete {
			return
		}
		if err := h.ApplySetupComplete(setupComplete); err != nil {
			log.Printf("apply setup state failed: %v", err)
			return
		}
		if setupComplete {
			log.Printf("registration setup complete for reader %s; analyzer communication enabled", cfg.Reader.ID)
		} else {
			log.Printf("registration setup incomplete for reader %s; analyzer communication disabled", cfg.Reader.ID)
		}
		_ = store.AppendEvent("registration_state_applied", map[string]interface{}{
			"setup_complete": setupComplete,
			"profile":        profile,
		})
	}
	defer commMgr.Stop()

	stop := make(chan struct{})
	go ws.Run(stop)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	<-sigs
	close(stop)
	log.Printf("reader-agent shutdown")
	return nil
}
