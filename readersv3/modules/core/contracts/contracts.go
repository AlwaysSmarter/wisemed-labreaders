package contracts

import coremodel "wisemed-labreaders/readersv3/modules/core/model"

type ReaderConfig interface {
	Save() error
}

type ReaderService interface {
	StatusSnapshot() map[string]interface{}
	ConfigSnapshot() (interface{}, error)
	ConfigSection(section string) (interface{}, error)
	UpdateConfig(patch map[string]interface{}) error
	UpdateConfigSection(section string, value interface{}) error
	ListLogs(limit int) ([]coremodel.EventLog, error)
	DashboardSnapshot(limit int) (map[string]interface{}, error)
	StatsForDate(orderDate string) (map[string]interface{}, error)
	StatsSeries(limit int) (map[string]interface{}, error)

	ListAnalytes() ([]coremodel.Analyte, error)
	GetAnalyteByID(id int64) (coremodel.Analyte, error)
	SaveAnalyte(item coremodel.Analyte) (int64, error)
	DeleteAnalyte(id int64) error

	ListOrderBundles(roundNo int, orderDate string) ([]coremodel.OrderBundle, error)
	ListOrders(roundNo int, orderDate string) ([]coremodel.Order, error)
	UpsertOrder(order coremodel.Order) (coremodel.Order, error)
	ListOrderAnalyses(orderID int64) ([]coremodel.OrderAnalysis, error)
	GetOrderAnalysis(id int64) (coremodel.OrderAnalysis, error)
	SaveOrderAnalysis(item coremodel.OrderAnalysis) (coremodel.OrderAnalysis, error)
	DeleteOrderAnalysis(id int64) error
	SetDefaultResult(orderAnalysisID, resultID int64, repeatMode string) error
	ListRoundNumbers(orderDate string) ([]int, error)
	CreateNextRound(orderDate string) (int, error)
	ImportFileNow(path, orderDate string) (ImportSummary, error)
	ExportOrdersCSV(orderIDs []int64, orderDate string) (string, int, error)

	ListQCRecordBundles(roundNo int, runDate string) ([]coremodel.QCRecordBundle, error)
	ListQCRecords(roundNo int, runDate string) ([]coremodel.QCRecord, error)
	UpsertQCRecord(item coremodel.QCRecord) (coremodel.QCRecord, error)
	ListQCAnalyses(recordID int64) ([]coremodel.QCAnalysis, error)
	GetQCAnalysis(id int64) (coremodel.QCAnalysis, error)
	SaveQCAnalysis(item coremodel.QCAnalysis) (coremodel.QCAnalysis, error)
	DeleteQCAnalysis(id int64) error
	ListQCRoundNumbers(runDate string) ([]int, error)
	CreateNextQCRound(runDate string) (int, error)
	ListQCTargets() ([]coremodel.QCTarget, error)
	GetQCTarget(id int64) (coremodel.QCTarget, error)
	SaveQCTarget(item coremodel.QCTarget) (coremodel.QCTarget, error)
	DeleteQCTarget(id int64) error
	QCPerformance(analyteTag, controlLevel, lotNo, dateFrom, dateTo string, limit int) (map[string]interface{}, error)
}

type ImportSummary struct {
	FileName  string
	Imported  int
	Warnings  int
	Protocol  string
	Manual    bool
	OrderDate string
}
