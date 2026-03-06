package config

type ProtocolHandler interface {
	StartCommunication(ac AnalyzerConnection)
	SendData(ac AnalyzerConnection, data []byte)
	SendString(ac AnalyzerConnection, data string)
	TestCommunication(ac AnalyzerConnection, data string)
	ParseCluster(ac AnalyzerConnection, data string)
	InitiateCommand(ac AnalyzerConnection, cmd string, args ...interface{})
	OnDataArrived(ac AnalyzerConnection, data string)
	OnSetInitialized(ac AnalyzerConnection, initialized bool)
}

type AnalyzerConnection interface {
	GetConnId() string
	SendData(data []byte)
	SendString(data string)
}
