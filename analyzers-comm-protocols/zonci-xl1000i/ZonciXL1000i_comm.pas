unit ZonciXL1000i_comm;
// threading -ok
//


interface

uses SysUtils, Classes, Types, Windows, SyncObjs, ExtCtrls, StrUtils, SerialNG, Dialogs,
  ScktComp, Contnrs, DateUtils, forms, Graphics,
  u_CIFCommObj, u_CIFUtils   ;

const
  FullLIS2 = True;                    // LIS2 fields padded with ^
  SenderName = 'HOST';
  ReceiverName = 'SELXL^2.1.0';
  ENQ = #5;
  SOH = #1;
  STX = #2;
  ETX = #3;
  EOT = #4;
  ACK = #6;
  NAK = #21;
  ETB = #23;

  LF  = #10;
  CR  = #13;
  SP  = #32;
  TAB = #9;

type
  ECEInvalidHeader = Exception;

  TZonciXL1000iPatientResult = class (TPatientResult)
 public
  end;

  TCommunicationType = (ctSerial, ctTcpIP);


  TCommThread = class(TThread)
  private
    FActive: Boolean;
    CS: TCriticalSection;
    IsInitialized: Boolean;
    f_DataArrived: Boolean;
    Sock : TServerSocket;
    ClientSock : TCustomWinSocket;
    comm : TSerialPortNG;
    bHadSTX, bHadETX  : Boolean;
    FBufferText : AnsiString;
    DataBuffer : TObjectList;
    Owner : TObject;

    FOnDataArrived : TNotifyEvent;
    FIsCommunication : boolean;
    FOnOutputDebugMessage : TOutputDebugMessage;
    FOrderEntryConfirmation : TOrderEntryConfirmation;
    FPrepareOrderInformation : TPrepareOrderInformation;
    FPrepareOrderInformationAll : TPrepareOrderInformationAll;
    FDebugMessage : AnsiString;
    FCommunicationType : TCommunicationType;

    FNoHandShake : boolean;                // NoHandShake = true - LIS2 in mod bloc
    FCurrentPatient : TPatientResult;
    FPatientOrder : TPatientResult;        // used to synchronize orderentryconfirmation

    SendStr_queue : TStringList;
    SendOrd_queue : TObjectList;
    SendOrdSeg_queue : TObjectList;
    OrdReq_queue : TObjectList;
    Res_queue : TObjectList;
    ResOrd_queue : TObjectList;
    BufferText: AnsiString; //processing purposes for non often reallocation

    // timeout timer; only private
    FtimTimeOut : TTimer;
    CheckTimeout : Boolean;
    ResidualData : AnsiString;

    procedure timTimeOutTimer(Sender: TObject);

    procedure ThreadTerminate(Sender: TObject);
    procedure SocketRead(Sender: TObject;
  Socket: TCustomWinSocket);
    procedure InqSocketRead(Sender: TObject;
  Socket: TCustomWinSocket);
    procedure SendString(AString:AnsiString);

    procedure CommRxClusterEvent(Sender: TObject);

    procedure SetActive(AValue:Boolean);
    procedure SetInitialized(AValue:Boolean);
    procedure SetIsCommunication(AValue:Boolean);
    procedure ParseCluster(Data:AnsiString);
    procedure SendOrderAll;
    procedure ReceiveOrderRequestAll;
    procedure SaveResultsAll;
    procedure StoreDecodedData(myPatient:TObject);
    procedure StartBeacon;

    procedure OutputDebugMessage(AMessage:AnsiString);
    procedure DoOutputDebugMessage;
    procedure DoDataArrived;
    procedure DoSetIsCommunication;
    procedure DoOrderEntryConfirmation(PatientOrder:TPatientResult);
    procedure OrderEntryConfirmation;
    procedure SocketConnect(Sender: TObject; Socket: TCustomWinSocket);

  protected
    procedure Execute;override;

  public
    constructor Create(AOwner:TObject; ABuffer:TObjectList; ACS:TCriticalSection);
    destructor Destroy;override;

    procedure AddOrderEntry(PatientOrder:TPatientResult);
    procedure GetPatientResults(PatientOrder:TPatientResult);
    procedure AddOrderEntryBatchList(PatientsList : TObjectList);
    procedure TestReceive(Data:AnsiString);
    procedure TestRoutine(RoutineName:string);


  property Active: boolean read FActive write SetActive;
  property NoHandShake: Boolean read FNoHandShake write FNoHandShake;
  property CommunicationType: TCommunicationType read FCommunicationType write FCommunicationType;

  property OnOutputDebugMessage: TOutputDebugMessage read FOnOutputDebugMessage write FOnOutputDebugMessage;
  property OnDataArrived: TNotifyEvent read FOnDataArrived write FOnDataArrived;
  property OnPrepareOrderInformation: TPrepareOrderInformation read FPrepareOrderInformation write FPrepareOrderInformation;
  property OnPrepareOrderInformationAll: TPrepareOrderInformationAll read FPrepareOrderInformationAll write FPrepareOrderInformationAll;
  property OnOrderEntryConfirmation: TOrderEntryConfirmation read FOrderEntryConfirmation write FOrderEntryConfirmation;
  property IsCommunication: boolean read FIsCommunication write SetIsCommunication;

  end;

  TZonciXL1000i = class(TCIFCommObj)
  private
    FNoHandShake : boolean;                // NoHandShake = true - LIS2 in mod bloc

    CommBuffer, TempBuffer : TObjectList;
    CS: TCriticalSection;
    FCommThread : TCommThread;
    FCommTimer : TTimer;

    PNGPath : AnsiString;
    PNGGraphColor : TColor;


    procedure SetNoHandShake(Value: Boolean);
    procedure CommTimerTimer(Sender: TObject);

  protected
    procedure DoActive(Value: Boolean);override;
    procedure SetOnDataArrived(AProc:TOnDataArrived);override;
    procedure SetPrepareOrderInformation(AProc:TPrepareOrderInformation);override;
    procedure SetPrepareOrderInformationAll(AProc:TPrepareOrderInformationAll);override;
    procedure SetOrderEntryConfirmation(AProc:TOrderEntryConfirmation);override;
    procedure SetOnOutputDebugMessage(AProc:TOutputDebugMessage);override;

  public
    constructor Create(AOwner: TComponent); override;
    destructor Destroy; override;
    function AddOrderEntry(PatientOrder:TPatientResult):boolean;override;
    function AddOrderEntryBatchList(PatientsList : TObjectList):boolean;
    procedure GetPatientResults(PatientOrder:TPatientResult); override;
    procedure Test;override;
    procedure GetReagentInstallation; override;

  published
    property NoHandShake: Boolean read FNoHandShake write SetNoHandShake;
  end;



implementation

uses StringUtils;

{=============== Communication Layer ==========================================}

// ** Communication Thread ** //
constructor TCommThread.Create(AOwner : TObject; ABuffer:TObjectList; ACS:TCriticalSection);
Begin
  inherited Create(True);
  FreeOnTerminate := False;     // f. important
  OnTerminate := ThreadTerminate;
  Owner := AOwner;

    // select communication type
  if (param_str_has_option('TCPIP')) then
    CommunicationType := ctTcpIP
  else
    CommunicationType := ctSerial;


  if CommunicationType = ctTcpIP then
    begin
      Sock := TServerSocket.Create(nil);
      Sock.Port := 3125;
      Sock.ServerType := stNonBlocking;
      sock.OnClientRead := SocketRead;
      sock.OnAccept := SocketConnect;
      {}
    end
  else
    begin
      Comm := TSerialPortNG.Create(nil);
      with Comm do
        begin
          BaudRate := 115200;
          if ParamCount > 0 then
            CommPort := ParamStr(1)
          else
            CommPort := 'COM1';
          OutputDebugString(PChar(Format('%s PORT', [CommPort])));

          OnRxClusterEvent := CommRxClusterEvent;
          {$IFDEF iDebug}
          OnCommEvent := CommCommEvent;
          OnCommStat := CommCommStat;
          {$ENDIF}
        end;
  end;
  ClientSock := nil;
  FCurrentPatient := nil;

  SendStr_queue := TStringList.Create;
  Res_queue := TObjectList.Create;
  ResOrd_queue := TObjectList.Create;
  OrdReq_queue := TObjectList.Create;
  SendOrd_queue := TObjectList.Create;
  SendOrdSeg_queue := TObjectList.Create;

  ftimTimeOut := TTimer.Create(nil);
  FtimTimeOut.Interval := 2000;
  FtimTimeOut.OnTimer := timTimeOutTimer;

  FtimTimeOut.Enabled := True;

  CS := ACS;        // using global comm CS
  DataBuffer := ABuffer;    // buffer coada prin care trimit datele la main thread
  bHadSTX := False;
  bHadETX := False;
End;

destructor TCommThread.Destroy;
Begin
  SetActive(False);
  FreeAndNil(Sock);
  FreeAndNil(CS);

  FreeAndNil(SendStr_queue);
  FreeAndNil(Res_queue);
  FreeAndNil(ResOrd_queue);
  FreeAndNil(OrdReq_queue);
  FreeAndNil(SendOrd_queue);
  FreeAndNil(SendOrdseg_queue);
  inherited;
End;


procedure TCommThread.Execute;
Begin
    // nothing to do, yet
End;

procedure TCommThread.SetActive(AValue:Boolean);
Begin
  OutputDebugMessage('active');
  if Assigned(Sock)
    then Sock.Active := AValue;
  if Assigned(comm)
    then comm.Active := AValue;
//  Sock_Inq.Active := AValue;
End;

procedure TCommThread.ThreadTerminate;
Begin
  FreeAndNil(Sock);
End;

function ReadBuffer(Data:AnsiString;var pos:integer;delim:AnsiChar):AnsiString;
var buf : AnsiString;
Begin
    // citeste din buffer de la pozitia pos pana intalneste delim
    buf := '';
    while (Data[pos] <> delim) {and (pos < length(Data)){} do begin
        buf := buf + Data[pos];
        inc(pos);
    end;
    Result := buf;
End;

procedure TCommThread.StoreDecodedData(myPatient:TObject);
Begin
  // acceseaza databuffer, trimite datele decodificate
  CS.Acquire;
  DataBuffer.Add(myPatient);
  CS.Release;
End;

procedure TCommThread.SocketRead(Sender: TObject;
  Socket: TCustomWinSocket);
var s : Ansistring;
Begin
  CS.Acquire;


  SetLength(s, Socket.ReceiveLength);
  Socket.ReceiveBuf(Pointer(s)^,  Length(s));

  ParseCluster(s);
  CS.Release;
  Synchronize(DoDataArrived);
End;
procedure TCommThread.CommRxClusterEvent(Sender: TObject);
begin
  if Comm.NextClusterSize > 0 then
    begin
      CS.Acquire;
      ParseCluster(Comm.ReadNextClusterAsString);
      CS.Release;
      Synchronize(DoDataArrived);
    end;
end;


procedure TCommThread.SocketConnect(Sender: TObject; Socket: TCustomWinSocket);
Begin
   OutputDebugMessage('Connect');
   // blocking / nonblocking
   ClientSock := Socket;
End;

procedure TCommThread.InqSocketRead(Sender: TObject;
  Socket: TCustomWinSocket);
Begin
  CS.Acquire;
//  OnReceive('InquirySocket:' + Socket.ReceiveText);
  CS.Release;
End;

procedure TCommThread.SendString(AString:AnsiString);
Begin
  if ClientSock <> nil then begin
      ClientSock.SendText(AString);
      OutputDebugMessage('out:' + AString);
  end;

  if comm <> nil then begin
      Comm.SendString(AString);
      OutputDebugMessage('out:' + AString);
  end;
End;

procedure TCommThread.DoDataArrived;
Begin
  if Assigned(FOnDataArrived) then
    FOnDataArrived(Self);
End;

procedure TCommThread.SetIsCommunication;
Begin
  FIsCommunication := Avalue;

  Synchronize(DoSetIsCommunication);
End;

procedure TCommThread.DoSetIsCommunication;
Begin
  (Owner as TZonciXL1000i).IsCommunication := FIsCommunication;
End;

procedure TCommThread.DoOutputDebugMessage;
Begin
  if not Assigned(FOnOutputDebugMessage) then
      OutputDebugString(PChar(FDebugMessage))
  else
    FOnOutputDebugMessage(Self, FDebugMessage);
End;

procedure TCommThread.OutputDebugMessage(AMessage:AnsiString);
Begin
  FDebugMessage := AMessage;
  Synchronize(DoOutputDebugMessage); // thread safe console
End;


procedure TCommThread.SetInitialized(AValue:Boolean);
Begin
  if not AValue then
    IsInitialized := AValue
  else begin
    IsInitialized := AValue;
    f_DataArrived := False;
//    timTimeOut.Enabled := True;
  end;
End;

procedure TCommThread.ParseCluster(Data: AnsiString);
var Idx, spos, epos : Integer;
    tmpstr : ansistring;

  procedure CheckBuffer(myStr : AnsiString);
  var
    myPatient : TPatientResult;
    packageType: AnsiChar;
    dataString : AnsiString;
    lfPos, Idx : Integer;
    data_strings, work_list, details_list : TStringList;
  begin
       if (Length(myStr)<=0) then Exit;

       work_list := TStringList.Create;
       myPatient := nil;
       data_strings := TStringList.Create;
       details_list := TStringList.Create;

       packageType := myStr[1];



       case packageType of
           '1' :begin
                   //Query Test from HOST
                   lfPos := Pos(LF, myStr);

                   SplitString(SP, myStr, data_strings);
                   OutputDebugMessage(Format('QUERY BarCODE %s', [data_strings[1]]));

                   myPatient := TPatientResult.Create;
                   try
                     // not used in elecsys
                     myPatient.PatientID := data_strings[1];
                     if Assigned(OnPrepareOrderInformation) then
                       OnPrepareOrderInformation(Self, myPatient, SendOrd_queue);
                     // clean up
                     //Comm.SendString(STX+ETX);
                   finally
                     FreeAndNil(myPatient);
                   end;





                end;
          '2' :begin
                   //Send Test Reuest to HOST - should never get here
                end;
          '3' :begin
                   //Patient Result
                   lfPos := Pos(LF, myStr);

                   SplitString(SP, copy(myStr, 1, lfPos-1), data_strings);
                   SplitString(LF, copy(myStr, lfPos + 1), work_list);

                   myPatient := TPatientResult.Create;
                   myPatient.PatientID := data_strings[1];

                   for Idx := 0 to work_list.Count - 1 do
                     begin
                        SplitString(TAB, work_list[Idx], details_list);
                        if (details_list.count > 1) then
                          begin
                            myPatient.AnalisysNames.Add(details_list[0]);
                            myPatient.AnalisysResults.Add(details_list[1]);
                          end;
                     end;

                end;
          '4' :begin
                  //QC Result
                end;
       end;

       try
       if (myPatient <> nil) then
        begin
          //I have a new patient
          CS.Acquire;
          Res_queue.Add(myPatient);
          CS.Release;
        end;
       finally
         FreeAndNil(data_strings);
         FreeAndNil(details_list);
         FreeAndNil(work_list);
       end;
  end;
begin
  OutputDebugMessage(Format('===>%s', [Data]));
  BufferText := Format('%s%s', [BufferText, Data]);
  if Assigned(OnDataArrived) then
     OnDataArrived(Self);


  spos := pos(STX, BufferText);
  epos := pos(ETX, BufferText);

  while (spos>0) and (epos>0) and (epos > spos)   do
    begin
      tmpStr := Copy(BufferText, spos+1, epos - spos-1);
      BufferText := Copy(BufferText, epos+1);

      CheckBuffer(tmpStr);
      SaveResultsAll;

      spos := pos(STX,BufferText);
      epos := pos(ETX,BufferText);

    end;
end;

procedure TCommThread.SendOrderAll;
begin
//
end;


procedure TCommThread.ReceiveOrderRequestAll;
Begin
//
End;

procedure TCommThread.SaveResultsAll;
var mypac : TZonciXL1000iPatientResult;
    qcfileinfo, code : integer;
begin

while (Res_queue.Count > 0) do
  begin
    OutputDebugMessage('send results');
    mypac := TZonciXL1000iPatientResult.Create;
    myPac.CopyFrom (Res_queue[0] as TPatientResult);
    StoreDecodedData(mypac);
    //FreeAndNil(mypac);
    Res_queue.Delete(0);
  end;
end;

procedure TCommThread.AddOrderEntry(PatientOrder:TPatientResult);
Begin
  SendOrd_queue.Add(PatientOrder);
  // start astm order
  SendOrderAll;
End;

procedure TCommThread.GetPatientResults(PatientOrder:TPatientResult);
var send_str : string;
Begin
  // start astm order
   // start astm order
End;

procedure TCommThread.AddOrderEntryBatchList(PatientsList : TObjectList);
Begin
  // replace buffer
  FreeAndNil(SendOrd_queue);
  SendOrd_queue := PatientsList;
  // create astm messages
  SendOrderAll;
  // start sending astm messages
end;

procedure TCommThread.DoOrderEntryConfirmation(PatientOrder:TPatientResult);
Begin
  FPatientOrder := PatientOrder;
  Synchronize(OrderEntryConfirmation);
End;

procedure TCommThread.OrderEntryConfirmation;
Begin
  if Assigned(OnOrderEntryConfirmation) then
     OnOrderEntryConfirmation(Self, FPatientOrder);
End;

procedure TCommThread.TestReceive(Data:AnsiString);
Begin
  ParseCluster(Data);
End;

procedure TCommThread.StartBeacon;
Begin
   //
End;

procedure TCommThread.timTimeOutTimer(Sender:TObject);
Begin
  if CheckTimeout then begin
    // no communication
    IsCommunication := False;
  end
  else begin
    // begin check timout

    // changed logic: if send queue not empty restart send
    StartBeacon;
    CheckTimeout := True;
  end;

End;

procedure TCommThread.TestRoutine(RoutineName:string);
Begin
  if RoutineName = 'ReceiveOrderRequestAll'
     then ReceiveOrderRequestAll;
  if RoutineName = 'TestBeacon'
     then begin
       StartBeacon;
     end;
End;


{** ZonciXL1000i_LIS2 **}
constructor TZonciXL1000i.Create(AOwner: TComponent);
begin
  CommBuffer := TObjectList.Create;
  CommBuffer.OwnsObjects := False;      // let me manage objects
  TempBuffer := TObjectList.Create;
  TempBuffer.OwnsObjects := False;      // let me manage objects

  CS := TCriticalSection.Create;
  FCommThread := TCommThread.Create(Self, CommBuffer, CS);      // send global CS to thread

  FCommTimer := TTimer.Create(self);
  FCommTimer.Interval := 200;
  FCommTimer.OnTimer := CommTimerTimer;

  FCommTimer.Enabled := True;

  NoHandShake := False;

  inherited;
end;

destructor TZonciXL1000i.Destroy;
begin
  FCommThread.Terminate;
  FCommThread.Free;
  CommBuffer.Free;
  TempBuffer.Free;
  inherited;
end;

procedure TZonciXL1000i.SetOnDataArrived(AProc:TOnDataArrived);
Begin
  inherited;
  FCommThread.FOnDataArrived := AProc;
End;

procedure TZonciXL1000i.SetOnOutputDebugMessage(AProc:TOutputDebugMessage);
Begin
  inherited;
  FCommThread.OnOutputDebugMessage := AProc;
End;

procedure TZonciXL1000i.SetPrepareOrderInformation(AProc:TPrepareOrderInformation);
Begin
  inherited;
  FCommThread.OnPrepareOrderInformation := AProc;
End;

procedure TZonciXL1000i.SetPrepareOrderInformationAll(AProc:TPrepareOrderInformationAll);
Begin
  inherited;
  FCommThread.OnPrepareOrderInformationAll := AProc;
End;

procedure TZonciXL1000i.SetOrderEntryConfirmation(AProc:TOrderEntryConfirmation);
Begin
  inherited;
  FCommThread.OnOrderEntryConfirmation := AProc;
End;

procedure TZonciXL1000i.DoActive(Value: Boolean);
begin
  FActive := Value;
  FCommThread.Active := FActive;
end;

procedure TZonciXL1000i.SetNoHandShake(Value: Boolean);
begin
  if FNoHandShake = Value then Exit;
  FNoHandShake := Value;
  FCommThread.NoHandShake := FNoHandShake;
end;

procedure TZonciXL1000i.CommTimerTimer(Sender:TObject);
{* citeste de pe comm buffer si trimite la DB *}
var DataObject : TZonciXL1000iPatientResult;
Begin
  // citeste si goleste comm buffer
  CS.Acquire;

  while CommBuffer.Count > 0 do begin
    TempBuffer.Add(CommBuffer[0]);
    CommBuffer.Delete(0);
  end;
  CS.Release;

  // avem datele in tempbuffer
  while TempBuffer.Count > 0 do begin
    if (TempBuffer[0]=nil) then
      begin
        TempBuffer.Delete(0);
        continue;
      end;

    DataObject := TempBuffer[0] as TZonciXL1000iPatientResult;
    TempBuffer.Delete(0);

    try
      try
      if DataObject.QCFileInfo <> '' then begin
          if Assigned(OnQCResultReady) then
              OnQCResultReady(Self, DataObject);
      end
      else begin
          if Assigned(OnResultReady) then
              OnResultReady(Self, DataObject);

          if Assigned(OnGetWholePatient) then
              OnGetWholePatient(Self, DataObject);
      end;

      except on E:Exception
          do OutputDebugMessage('Eroare la salvare:' + E.Message);
          // continua sa trimita restul de pacienti
      end;
    finally
      DataObject.Free;
    end;
  end;
End;



function TZonciXL1000i.AddOrderEntry(PatientOrder:TPatientResult):boolean;
Begin
  FCommThread.AddOrderEntry(PatientOrder);
End;

function TZonciXL1000i.AddOrderEntryBatchList(PatientsList : TObjectList):boolean;
Begin
  FCommThread.AddOrderEntryBatchList(PatientsList);
End;


procedure TZonciXL1000i.GetPatientResults(PatientOrder:TPatientResult);
 Begin
  FCommThread.GetPatientResults(PatientOrder);
End;

procedure TZonciXL1000i.GetReagentInstallation;
Begin
//
End;

procedure TZonciXL1000i.Test;
var
  str_res : TStringList;
  i : integer;

Begin
                {
  str_res := TStringList.Create;
  str_res.LoadFromFile('test_order.txt');

  for i := 0 to str_res.Count-1 do
    fcommthread.TestReceive(copy(str_res[i], 2, 100));

  FCommThread.TestRoutine('ReceiveOrderRequestAll');

  str_res.Free;
                 {}

  FCommThread.TestReceive(#2'1H|\^&|||cobas^1|||||host|TSREQ^REAL|P|1'#$D'Q|1|^^                449169^0^5002^2^^S1^SC||ALL||||||||O'#$D'L|1|N'#$D#3'B1'#$D#$A);
  FCommThread.TestReceive(EOT);
  // test beacon
  FCommThread.iscommunication := false;
                 {}
End;



end.
