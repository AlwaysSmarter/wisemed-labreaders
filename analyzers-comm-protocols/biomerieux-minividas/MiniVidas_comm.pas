unit MiniVidas_comm;
// threading -ok
//


interface

uses SysUtils, Classes, Types, Windows, SyncObjs, ExtCtrls, StrUtils, SerialNG, Dialogs,
  ScktComp, Contnrs, DateUtils, forms, Graphics,
  stringUtils,
  u_CIFCommObj, u_CIFUtils   ;


const
  FullASTM = True;                    // ASTM fields padded with ^

type
  TMiniVidasPatientResult = class(TPatientResult)

  end;


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

    FOnDataArrived : TNotifyEvent;
    FOnOutputDebugMessage : TOutputDebugMessage;
    FOrderEntryConfirmation : TOrderEntryConfirmation;
    FPrepareOrderInformation : TPrepareOrderInformation;
    FPrepareOrderInformationAll : TPrepareOrderInformationAll;
    FDebugMessage : AnsiString;
    FResAsInterpretation: AnsiString;


    SendList : TStringList;

    procedure ThreadTerminate(Sender: TObject);
    procedure SocketRead(Sender: TObject;
  Socket: TCustomWinSocket);
    procedure InqSocketRead(Sender: TObject;
  Socket: TCustomWinSocket);
    procedure SendString(AString:AnsiString);

    procedure CommRxClusterEvent(Sender: TObject);

    procedure SetActive(AValue:Boolean);
    procedure SetInitialized(AValue:Boolean);
    procedure ParseCluster(Data:AnsiString);
    procedure StoreDecodedData(myPatient:TObject);
    procedure CheckBuffer(Data:AnsiString);
    function CalculateCRC(Data:AnsiString):AnsiString;

    procedure AddOrderEntryBatchList(OrdersList:TObjectList);


    procedure OutputDebugMessage(AMessage:AnsiString);
    procedure DoOutputDebugMessage;
    procedure DoDataArrived;
    procedure SocketConnect(Sender: TObject; Socket: TCustomWinSocket);


  protected
    procedure Execute;override;

  public
    constructor Create(ABuffer:TObjectList; ACS:TCriticalSection);
    destructor Destroy;override;

    procedure TestReceive(Data:AnsiString);
    procedure SendTest;

  property Active: boolean read FActive write SetActive;
  property OnOutputDebugMessage: TOutputDebugMessage read FOnOutputDebugMessage write FOnOutputDebugMessage;
  property OnDataArrived: TNotifyEvent read FOnDataArrived write FOnDataArrived;
  property OnPrepareOrderInformation: TPrepareOrderInformation read FPrepareOrderInformation write FPrepareOrderInformation;
  property OnPrepareOrderInformationAll: TPrepareOrderInformationAll read FPrepareOrderInformationAll write FPrepareOrderInformationAll;
  end;

  TMiniVidas = class(TCIFCommObj)
  private
    CommBuffer, TempBuffer : TObjectList;
    CS: TCriticalSection;
    FCommThread : TCommThread;
    FCommTimer : TTimer;

    PNGPath : AnsiString;
    PNGGraphColor : TColor;
    FResAsInterpretation: AnsiString;

    procedure CommTimerTimer(Sender: TObject);

  protected
    procedure DoActive(Value: Boolean);override;
    procedure SetOnDataArrived(AProc:TOnDataArrived);override;
    procedure SetPrepareOrderInformation(AProc:TPrepareOrderInformation);override;
    procedure SetPrepareOrderInformationAll(AProc:TPrepareOrderInformationAll);override;
    procedure SetOnOutputDebugMessage(AProc:TOutputDebugMessage);override;
    procedure SetResAsInterpretation(Value: AnsiString);

  public
    constructor Create(AOwner: TComponent); override;
    destructor Destroy; override;
    procedure SendPatient(patient:TPatientResult);
    procedure AddOrderEntryBatchList(OrdersList:TObjectList);override;
    procedure Test;override;
    property ResAsInterpretation: AnsiString read FResAsInterpretation write SetResAsInterpretation;

  published
  end;

const
  ENQ = #5;
  STX = #2;
  EOT = #4;
  EOF = #$1A;
  ETX = #3;
  ACK = #6;
  NAK = #15;
  RS = #30;
  GS = #29;


implementation


{=============== Communication Layer ==========================================}

// ** Communication Thread ** //
constructor TCommThread.Create(ABuffer:TObjectList; ACS:TCriticalSection);
Begin
  inherited Create(True);
  FreeOnTerminate := False;     // f. important
  OnTerminate := ThreadTerminate;

  {
  Sock := TServerSocket.Create(nil);
  Sock.Port := 15000;
  Sock.ServerType := stNonBlocking;
  sock.OnClientRead := SocketRead;
  sock.OnAccept := SocketConnect;
  {}

  Comm := TSerialPortNG.Create(nil);
  with Comm do
    begin
      BaudRate := 38400;
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

  ClientSock := nil;

  SendList := TStringList.Create;

  CS := ACS;        // using global comm CS
  DataBuffer := ABuffer;    // buffer coada prin care trimit datele la main thread
  bHadSTX := False;
  bHadETX := False;
  FBufferText := '';
End;

destructor TCommThread.Destroy;
Begin
  SetActive(False);
  FreeAndNil(Sock);
  FreeAndNil(CS);

  FreeAndNil(SendList);
  inherited;
End;


procedure TCommThread.Execute;
Begin
    // nothing to do, yet
End;

procedure TCommThread.SetActive(AValue:Boolean);
Begin
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
Begin
  CS.Acquire;
  ParseCluster(Socket.ReceiveText);
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
  OutputDebugMessage('SendString');
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

procedure TCommThread.CheckBuffer(Data:AnsiString);
var StrFile,AnalisysInterp : TStringList;
    decoded : boolean;
    Buffer, field_type : AnsiString;
    myPatient : TPatientResult;
    i : integer;
    mdy, prefix, actualData, tmpanname : string;
Begin
    StrFile := TStringList.Create;
    AnalisysInterp := TStringList.Create;
    SplitString('|', FResAsInterpretation, AnalisysInterp);
    SplitString('|', Data,StrFile);

    myPatient := TPatientResult.Create;

    for i := 0 to StrFile.Count-1 do
      begin
        if StrFile[i][1] = #$1E then StrFile[i] := Copy(StrFile[i], 2, length(StrFile[i]));
        
        actualData := trim(copy(StrFile[i], 3, Length(StrFile[i])));
        prefix := trim(Copy(StrFile[i], 1, 2));
        if prefix = 'ci' then
          myPatient.PatientID := remove_leading_zeros(OnlyNumbers(actualData))
        else
        if prefix = 'rt' then
          myPatient.AnalisysNames.Add(actualData)
        else
        if prefix = 'tt' then
          myPatient.ResultTime := actualData
        else
        if prefix = 'td' then
          myPatient.ResultDate := actualData
        else
        begin
          if (myPatient.AnalisysNames.count > 0) then
            tmpanname := myPatient.AnalisysNames.Strings[myPatient.AnalisysNames.count-1]
          else
            tmpanname := '-1';

          if (AnalisysInterp.IndexOf(tmpanname) < 0) then
            begin
              if prefix = 'qn' then
                myPatient.AnalisysResults.Add(remove_leading_zeros(OnlyNumbersAndPunctuation(actualData)))
            end
          else
            begin
              if prefix = 'ql' then
                myPatient.AnalisysResults.Add(remove_leading_zeros(actualData))
            end;
        end;
      end;
    outputDebugMessage('decoded patient:' + myPatient.alltostring);
    decoded := True;
    StrFile.Free;

    if decoded then begin
       StoreDecodedData(myPatient);
    end
    else
      FreeAndNil(myPatient);  // pt. siguranta

    FBufferText := '';
    bHadSTX := false;
    bHadETX := false;
End;

procedure TCommThread.ParseCluster(Data:AnsiString);
{*  machine protocol *}
var i : integer;
Begin
  OutputDebugMessage(Format('===>[%d] %s', [Ord(Data[1]), Data]));

  if Assigned(FOnDataArrived) then
        FOnDataArrived(Self);{}

  f_DataArrived := True;

  FBufferText := Format('%s%s', [FBufferText, Data]);

  if Pos(EOT, FBufferText) > 0 then begin
    OutputDebugMessage('Have EOT');
    CheckBuffer(Copy(FBufferText, 1, Pos(EOT, FBufferText)));
    FBufferText := Copy(FBufferText, Pos(EOT, FBufferText)+1, Length(FBufferText)-Pos(EOT, FBufferText));
  end
  else
  OutputDebugMessage('NO EOT yet');

  if (Data[1]<>EOT) then sendString(ACK);

End;

procedure TCommThread.AddOrderEntryBatchList(OrdersList:TObjectList);
Begin
End;


function TCommThread.CalculateCRC(Data:AnsiString):AnsiString;
Begin
End;

procedure TCommThread.SendTest;
var str : AnsiString;
Begin
  OutputDebugMessage('send test:');
  str := '"1125","       23458","Smith 123       ",0,3,';
  sendString(STX + str + CalculateCRC(str) +  ETX);
End;


procedure TCommThread.TestReceive(Data:AnsiString);
Begin
  ParseCluster(Data);
End;


{** ABXPentra120_ASTM **}
constructor TMiniVidas.Create(AOwner: TComponent);
begin
  CommBuffer := TObjectList.Create;
  CommBuffer.OwnsObjects := False;      // let me manage objects
  TempBuffer := TObjectList.Create;
  TempBuffer.OwnsObjects := False;      // let me manage objects

  CS := TCriticalSection.Create;
  FCommThread := TCommThread.Create(CommBuffer, CS);      // send global CS to thread


  FCommTimer := TTimer.Create(self);
  FCommTimer.Interval := 200;
  FCommTimer.OnTimer := CommTimerTimer;

  FCommTimer.Enabled := True;

  NoHandShake := False;
  
  inherited;
end;

destructor TMiniVidas.Destroy;
begin
  FCommThread.Terminate;
  FCommThread.Free;
  CommBuffer.Free;
  TempBuffer.Free;
  inherited;
end;

procedure TMiniVidas.SetResAsInterpretation(Value: AnsiString);
begin
  FResAsInterpretation := Value;
  FCommThread.FResAsInterpretation := Value;
end;
procedure TMiniVidas.SetOnDataArrived(AProc:TOnDataArrived);
Begin
  inherited;
  FCommThread.FOnDataArrived := AProc;
End;

procedure TMiniVidas.SetOnOutputDebugMessage(AProc:TOutputDebugMessage);
Begin
  inherited;
  FCommThread.OnOutputDebugMessage := AProc;
End;

procedure TMiniVidas.SetPrepareOrderInformation(AProc:TPrepareOrderInformation);
Begin
  inherited;
  FCommThread.OnPrepareOrderInformation := AProc;
End;

procedure TMiniVidas.SetPrepareOrderInformationAll(AProc:TPrepareOrderInformationAll);
Begin
  inherited;
  FCommThread.OnPrepareOrderInformationAll := AProc;
End;

procedure TMiniVidas.DoActive(Value: Boolean);
begin
  FActive := Value;
  FCommThread.Active := FActive;
end;

procedure TMiniVidas.CommTimerTimer(Sender:TObject);
{* citeste de pe comm buffer si trimite la DB *}
var DataObject : TPatientResult;
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
    DataObject := TempBuffer[0] as TPatientResult;
    TempBuffer.Delete(0);

    try
      try
      if DataObject.is_qc then begin
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

procedure TMiniVidas.AddOrderEntryBatchList(OrdersList:TObjectList);
Begin
  FCommThread.AddOrderEntryBatchList(OrdersList);
End;


procedure TMiniVidas.SendPatient(patient:TPatientResult);
Begin

End;

procedure TMiniVidas.Test;
var
  str_res : TStringList;
  str : AnsiString;
Begin
  str_res := TStringList.Create;
  str_res.LoadFromFile('TEST.LOG');

  str := #02#30'mtrsl|pi2419|pnBotezatu Diana|si|ci2419|rtHCV|rnAnti-HCV|tt12:34|td03/03/20|qlNe'#30'gative|qn0.06|idVIDAS3PC01|'#29'4e'#13#10#3#4#13#10;
  fcommthread.TestReceive(str);

  str_res.Free;
End;
end.
