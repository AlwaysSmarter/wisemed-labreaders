package tcpip

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
	"wisemed-labreaders/config"
	"wisemed-labreaders/general"
)

type Server struct {
	listener    net.Listener
	quit        chan interface{}
	wg          sync.WaitGroup
	ch          config.ProtocolHandler
	cph         config.CreateProtocolHandler
	commStarted bool
}

var TCPIPReadDealineItmeout = 200 * time.Millisecond
var TCPIPWriteDealineItmeout = 200 * time.Millisecond

var tcpipConnections = general.ObjectQueue{}

var tcpipServer *Server = nil

type TCPIPConnection struct {
	connChannel net.Conn
}

func IsCommunicationActive() bool {
	if tcpipServer != nil {
		return true
	}
	return false
}

func (tc TCPIPConnection) GetConnId() string {
	return fmt.Sprintf("%s", tc.connChannel.RemoteAddr().String())
}

func (tc TCPIPConnection) SendData(data []byte) {
	if tc.connChannel != nil {
		fmt.Printf("\n<---- %q\n", string(data))
		bytes, err := tc.connChannel.Write(data)
		if err != nil {
			fmt.Println("TCP-WRITE error ", err)
		} else {
			fmt.Printf("\nTCP-WRITE written %d bytes", bytes)
		}
	}
}

func (tc TCPIPConnection) SendString(data string) {
	tc.SendData([]byte(data))
}

func InitiateCommand(createProtoHandler config.CreateProtocolHandler, cmd string, args ...interface{}) (int, error) {
	sentTo := 0
	tcpipConnections.Lock()
	for idx := 0; idx < len(tcpipConnections.Data); idx++ {
		conn := tcpipConnections.Data[idx]
		tcpipServer.ch.InitiateCommand(&TCPIPConnection{conn.(net.Conn)}, cmd, args...)
		sentTo++
	}
	tcpipConnections.UnLock()
	return sentTo, nil
}
func TestCommunication(createProtoHandler config.CreateProtocolHandler, cmd string, args ...interface{}) error {
	myCh := tcpipServer.ch
	if myCh == nil {
		myCh = tcpipServer.cph()
	}
	myCh.TestCommunication(nil, cmd)
	return nil
}

func BroadcastMessage(createProtoHandler config.CreateProtocolHandler, msg string) error {
	tcpipConnections.Lock()
	for idx := 0; idx < tcpipConnections.Len(); idx++ {
		conn, err := tcpipConnections.GetObject(idx)
		if err != nil {
			conn.(net.Conn).Write([]byte(msg))
		}
	}
	tcpipConnections.UnLock()
	return nil
}
func EndTCPIPCommunication() {
	if tcpipServer != nil {
		fmt.Println("Stopping TCPIP communication")
		for idx := 0; idx < tcpipConnections.Len(); idx++ {
			conn, err := tcpipConnections.GetObject(idx)
			if err != nil {
				conn.(net.Conn).Close()
			}
		}
		tcpipConnections.Clear()
		tcpipServer.Stop()
		//to be sure is down
		time.Sleep(201 * time.Millisecond)
		fmt.Println("TCPIP communication stopped")
		tcpipServer = nil
	}
}
func StartTCPIPCommunication(createProtoHandler config.CreateProtocolHandler) error {
	EndTCPIPCommunication()
	if config.ServerConfiguration.CommTCPIPPort == "" ||
		config.ServerConfiguration.CommTCPIPAddress == "" ||
		config.ServerConfiguration.CommTCPIPType == "" {
		return errors.New(fmt.Sprintf("TCP/IP settings missing\nType: %s\nPort: %s\nAddress: %s", config.ServerConfiguration.CommTCPIPType, config.ServerConfiguration.CommTCPIPAddress, config.ServerConfiguration.CommTCPIPPort))
	}
	tcpipConnections.Clear()
	tcpipServer = &Server{
		quit: make(chan interface{}),
		cph:  createProtoHandler,
	}
	listenAddr := fmt.Sprintf("%s:%s", config.ServerConfiguration.CommTCPIPAddress, config.ServerConfiguration.CommTCPIPPort)
	fmt.Println("Starting TCP/IP communication on " + listenAddr)
	l, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Println("Error:", err)
		return err
	}
	tcpipServer.listener = l
	tcpipServer.wg.Add(1)
	go tcpipServer.serve()

	return nil
}

func (s *Server) Stop() {
	close(s.quit)
	s.listener.Close()
	s.wg.Wait()
}

func (s *Server) serve() {
	defer s.wg.Done()

	for {
		log.Println("Serving ...")

		conn, err := s.listener.Accept()
		log.Println("New CONNECTION")
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				log.Println("accept error", err)
			}
		} else {
			s.wg.Add(1)
			go func() {
				s.handleConection(conn)
				s.wg.Done()
			}()
		}
	}
}

func (s *Server) handleConection(conn net.Conn) {
	defer conn.Close()
	tcpConnIdx := tcpipConnections.PushWithIdx(conn)
	buf := make([]byte, 2048)
	s.ch = s.cph()
	s.ch.StartCommunication(&TCPIPConnection{conn})
ReadLoop:
	for {
		select {
		case <-s.quit:
			_, err := tcpipConnections.PopIdx(tcpConnIdx)
			if err != nil {
				log.Println("Cannot delete connection from active connections list", err)
			}
			return
		default:
			conn.SetReadDeadline(time.Now().Add(TCPIPReadDealineItmeout))
			conn.SetWriteDeadline(time.Now().Add(TCPIPWriteDealineItmeout))

			n, err := conn.Read(buf)
			if err != nil {
				if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
					continue ReadLoop
				} else if err != io.EOF {
					log.Println("read error", err)
					return
				}
			}
			//log.Println(fmt.Sprintf("READ %d bytes %v", n, buf[0:n]))
			if n == 0 {
				return
			}

			s.ch.ParseCluster(&TCPIPConnection{conn}, string(buf[0:n]))
		}
	}
}
