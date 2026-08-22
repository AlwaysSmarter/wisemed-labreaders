unit abxpentra400_comm;
// threading -ok
//


interface

uses SysUtils, Classes, Types, Windows, SyncObjs, ExtCtrls, StrUtils, SerialNG, Dialogs,
  ScktComp, Contnrs, DateUtils, forms, Graphics, ASTMUnit,
  u_CIFCommObj, u_CIFUtils   ;

const
  FullASTM = True;                    // ASTM fields padded with ^
  SenderName = 'ABX';
  ReceiverName = 'ABXP400';
type
  ECEInvalidHeader = Exception;

  TABXPentra400PatientResult = class (TPatientResult)
 public
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
    Owner : TObject;

    FOnDataArrived : TNotifyEvent;
    FIsCommunication : boolean;
    FOnOutputDebugMessage : TOutputDebugMessage;
    FOrderEntryConfirmation : TOrderEntryConfirmation;
    FPrepareOrderInformation : TPrepareOrderInformation;
    FPrepareOrderInformationAll : TPrepareOrderInformationAll;
    FDebugMessage : AnsiString;

    FNoHandShake : boolean;                // NoHandShake = true - ASTM in mod bloc
    FCurrentPatient : TPatientResult;
    FPatientOrder : TPatientResult;        // used to synchronize orderentryconfirmation

    SendStr_queue : TStringList;
    SendOrd_queue : TObjectList;
    SendOrdSeg_queue : TObjectList;
    OrdReq_queue : TObjectList;
    Res_queue : TObjectList;
    ResOrd_queue : TObjectList;

    // timeout timer; only private
    FtimTimeOut : TTimer;
    CheckTimeout : Boolean;
    ResidualData : AnsiString;
    ShouldSendEOT : Boolean;


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
    procedure ParseBlock(Data:AnsiString);
    function ParseRecord(Data:AnsiString):boolean;
    procedure ReceiveSegment(ASTMRec:TASTMSegment);
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
    procedure AddOrderEntryBatchList(PatientsList : TObjectList);
    procedure TestReceive(Data:AnsiString);
    procedure TestRoutine(RoutineName:string);

  property Active: boolean read FActive write SetActive;
  property NoHandShake: Boolean read FNoHandShake write FNoHandShake;
  property OnOutputDebugMessage: TOutputDebugMessage read FOnOutputDebugMessage write FOnOutputDebugMessage;
  property OnDataArrived: TNotifyEvent read FOnDataArrived write FOnDataArrived;
  property OnPrepareOrderInformation: TPrepareOrderInformation read FPrepareOrderInformation write FPrepareOrderInformation;
  property OnPrepareOrderInformationAll: TPrepareOrderInformationAll read FPrepareOrderInformationAll write FPrepareOrderInformationAll;
  property OnOrderEntryConfirmation: TOrderEntryConfirmation read FOrderEntryConfirmation write FOrderEntryConfirmation;
  property IsCommunication: boolean read FIsCommunication write SetIsCommunication;

  end;

  TABXPentra400 = class(TCIFCommObj)
  private
    FNoHandShake : boolean;                // NoHandShake = true - ASTM in mod bloc

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
    procedure Test;override;

  published
    property NoHandShake: Boolean read FNoHandShake write SetNoHandShake;
  end;

const
  ENQ = #5;
  STX = #2;
  EOT = #4;
  EOF = #$1A;
  ETX = #3;
  ACK = #6;
  NAK = #15;


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
  ShouldSendEOT := False;


   // select communication type

  if (param_str_has_option('TCPIP')) then
    begin
      Sock := TServerSocket.Create(nil);
      Sock.Port := 15000;
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
      OutputDebugMessage(Format('out: [%d] %s', [Ord(AString[1]), AString]));
  end;

  if comm <> nil then begin
      Comm.SendString(AString);
      OutputDebugMessage(Format('out: [%d] %s', [Ord(AString[1]), AString]));
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
  (Owner as TABXPentra400).IsCommunication := FIsCommunication;
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

procedure TCommThread.ParseBlock(Data:AnsiString);
var str_list : TStringList;
    i : integer;
Begin
  str_list := TStringList.Create;
  str_list.Text := Data;

  // indreapta segmentul O care face un line-break fara sa trebuiasca
  for i := 0 to str_list.Count-1 do begin
    if str_list[i][1] = 'O' then begin
      if str_list[i+1][1] <> 'R' then
          str_list[i] := str_list[i] + str_list[i+1];
      break;
    end;
  end;

  // go parse
  for i := 0 to str_list.Count-1 do
    ParseRecord(str_list[i]);

  str_list.Free;
End;

function TCommThread.ParseRecord(Data:AnsiString):boolean;
var ASTMRec : TASTMSegment;
    idx_SegName, idx : integer;
    wsData : string;      // widestring
    packagesList : TStringList;
    myData : AnsiString;
Begin
    Result := False;
    packagesList := TStringList.Create;

    SplitString(#$D, Data, packagesList);
    for idx := 0 to packagesList.Count - 1 do
      begin
        ResidualData := '';
        myData := packagesList.Strings[idx];
        wsData := String(myData);
        idx_SegName := 1;
        ASTMRec := nil;
        OutputDebugMessage('Parse Record:' + myData[idx_SegName]);
        try
        case myData[idx_SegName] of
        'H' : begin  // header
                if ((idx<packagesList.Count - 1) or (Length(wsData)>15)) then
                  ASTMRec := TASTMHeaderSegment.Create;
        end;
        'P' : begin  // Patient
                if ((idx<packagesList.Count - 1) or (Length(wsData)>15)) then
                ASTMRec := TASTMPIDSegment.Create;
        end;
        'O' : begin  // order  - asta e fapt il trimit eu
                if ((idx<packagesList.Count - 1) or (Length(wsData)>50)) then
                ASTMRec := TASTMTORSegment.Create;
        end;
        'Q' : begin  // order request
                if ((idx<packagesList.Count - 1) or (Length(wsData)>2)) then
                ASTMRec := TASTMRIRSegment.Create;
        end;
        'R' : begin  // result
                if ((idx<packagesList.Count - 1) or (Length(wsData)>15)) then
                ASTMRec := TASTMResRecSegment.Create;
        end;
        'C' : begin  // result
                if ((idx<packagesList.Count - 1) or (Length(wsData)>2)) then
                ASTMRec := TASTMCSegment.Create;
        end;
        'L' : begin  // termination
                if ((idx<packagesList.Count - 1) or (Length(wsData)>4)) then
                ASTMRec := TASTMMTRSegment.Create;
        end;
        'M' : begin  // termination
                if ((idx<packagesList.Count - 1) or (Length(wsData)>2)) then
                ASTMRec := TASTMMIRSegment.Create;
        end;
        end;

        if ASTMRec <> nil then begin

            ASTMRec.ParseASTMSegmentFromString(wsData);
            Result := True;
            ReceiveSegment(ASTMRec);
        end
        else
        begin
            ResidualData := myData;
            Result := True;
        end;
        except // eat exception -> result = false;
            on E:Exception do OutputDebugMessage('Eroare neidentificata:'+E.Message);
        end;
      end;
End;

procedure TCommThread.ReceiveSegment(ASTMRec:TASTMSegment);
var str_list : TStringList;
Begin
//  OutputDebugMessage('received segment:' + ASTMRec.SegmentName[1]);
  case ASTMRec.SegmentName[1] of
  'P' : begin
            // save last patient decoded
            //      * for case H, P, O, R, P, O, R, L
            if true {NoHandshake{} then begin
              if FCurrentPatient <> nil then
                  if Res_queue.Count > 0 then
                      SaveResultsAll;
            end;

            FreeAndNil(FCurrentPatient);        // ca sa fim siguri ca eliminam memory leaks
            // create new Patient, get id if existing
            FCurrentPatient := TPatientResult.Create;
            // try first id
            FCurrentPatient.PatientID := (ASTMRec as TASTMPIDSegment).PracticePatID;
            // try next id
            if FCurrentPatient.PatientID = '' then
              FCurrentPatient.PatientID := (ASTMRec as TASTMPIDSegment).LabPatID;
            FCurrentPatient.PatientName := (ASTMRec as TASTMPIDSegment).Name;
            if Copy((ASTMRec as TASTMPIDSegment).Name,1, 2) = '~C' then
              begin
                FCurrentPatient.is_qc := true;
                FCurrentPatient.QCFileInfo := Trim(Copy((ASTMRec as TASTMPIDSegment).Name,3, 6));
                FCurrentPatient.QCLotInfo := Trim(Copy((ASTMRec as TASTMPIDSegment).Name,9, 3));
                FCurrentPatient.QCLevelInfo := Trim(Copy((ASTMRec as TASTMPIDSegment).Name,18, 1));
              end;
            ASTMRec.Free;
        end;
  'Q' : begin
            // push ormder request in job queue
            OrdReq_queue.Add(ASTMRec);
        end;
  'O' : begin
            // store order information (fileID) - ResOrd_q si Res_q merg in paralel (se asociaza dupa index in lista)
            OutputDebugMessage('SegReceived order');

            if FCurrentPatient <> nil then
            begin
              {
                FOR CONTROLS:

                First 2 characters: ~C (use uppercase letter)
                Next 6 characters: Control Name (if fewer than 6 characters, right-padded with
                spaces; should not be empty)
                Next 3 characters: Control Lot (use 3 numeric digits; should not be empty)
                Next 6 characters: Expiration Date (use YYYYMM format; should not be empty)
                Last 1 character: Control Level (should not be empty)
              }
              if Copy((ASTMRec as TASTMTORSegment).SampleID,1, 2) = '~C' then
                begin
                  FCurrentPatient.is_qc := true;
                  FCurrentPatient.QCFileInfo := Trim(Copy((ASTMRec as TASTMTORSegment).SampleID,3, 6));
                  FCurrentPatient.QCLotInfo := Trim(Copy((ASTMRec as TASTMTORSegment).SampleID,9, 3));
                  FCurrentPatient.QCLevelInfo := Trim(Copy((ASTMRec as TASTMTORSegment).SampleID,18, 1));
                end;
              //if (FCurrentPatient.PatientID='') then
                begin
                  FCurrentPatient.PatientID := (ASTMRec as TASTMTORSegment).SampleID;
                end;
             end;
            ASTMRec.Free;
        end;
  'R' : begin
            // push result record in job queue
            Res_queue.Add(ASTMRec);
        end;
  'L' : begin
            // Save result in interactive mode and last result in block mode
            if Res_queue.Count > 0 then
                SaveResultsAll;
        end;
  else  ASTMRec.Free;
  end;
End;



procedure TCommThread.ParseCluster(Data:AnsiString);
{*  machine protocol *}
Begin
  OutputDebugMessage('===>' + Data);

  if Assigned(FOnDataArrived) then
        FOnDataArrived(Self);{}
  f_DataArrived := True;

  // reset timeout timer
  FtimTimeOut.Enabled := False;
  FtimTimeOut.Enabled := True;
  CheckTimeOut := False;
  IsCommunication := True;

  //ParseRecord(Data);            // numai pt testare
   Data:=Copy(Data, 1, 2)+ResidualData+Copy(Data, 3, Length(Data));
   ResidualData := '';

   OutputDebugMessage('Parsing: ===>' + Data);
  // parse bloc in mod nohandshake
  if NoHandShake then begin
    ParseBlock(Data);
    exit;
  end;

  if Data[1] = NAK then
    OutputDebugMessage(' got NAK');

  // ASTM interactiv
  case Data[1] of

  ENQ : Begin
        OutputDebugMessage('got ENQ');
        ASTMUnit.ASTM_prevSeqNo := 0;
        SendString(ACK);
        SetInitialized(True);       // check initialized on start communication
  end;

  STX : begin
     OutputDebugMessage('got STX');
          Data := Copy(Data, 3, Length(Data)-7);  // am scos STX si CR ETX C1 C2 CR LF
          if (Data[1]=#$D) then Data := Copy(Data, 2, Length(Data));

                        // eventual de implementat checksum

          if ParseRecord(Data) then begin
              SendString(ACK)
          end
          else begin
              OutputDebugMessage('send: NAK');
              SendString(NAK);
          end;
        end;
  ACK : begin    // raspund la ack -ul lui !! trebuie implementat si NAK
          OutputDebugMessage('got ACK');
{
              if SendStr_queue.Count > 0 then begin
                    OutputDebugMessage('sendsstr_queue: ' + SendStr_queue[0]);

                    SendString(SendStr_queue[0]);
                    SendStr_queue.Delete(0);
              end
              else SendString(EOT);
 {}
               if SendOrdSeg_queue.Count > 0 then
                 begin
                   ShouldSendEOT := true;
                   SendString((SendOrdSeg_queue[0] as TASTMSegment).GetASTMSegment);
                   SendOrdSeg_queue.Delete(0);
                 end
              else
                if (ShouldSendEOT) then
                  begin
                    SendString(EOT);
                    ShouldSendEOT := false;
                    OutputDebugMessage('sent EOT');
                  end;

        end;
  NAK : Begin
            //showmessage('Eroare de comunicatie - NAK !');
            OutputDebugMessage('Eroare de comunicatie - got NAK !');
            SendStr_queue.Clear;
            SendOrdSeg_queue.Clear;
  end;

  EOT : begin   // raspund la request-ul lui (Q )
            SetInitialized(False);
            // goleste job queue
            OutputDebugMessage('got EOT');
            if OrdReq_queue.Count > 0 then begin
                OutputDebugMessage('ordreq_queue');
                ReceiveOrderRequestAll;
            end;

            if SendOrd_queue.Count > 0 then begin
                OutputDebugMessage('sendord_queue');
                SendOrderAll;
            end;

            if SendOrdSeg_queue.Count > 0 then begin
                SendString(ENQ);
                ASTMUnit.ASTM_prevSeqNo := 0;
                OutputDebugMessage('sendordSeg_queue ENQ');
            end;
            //SendString(ENQ);
            //else
            //   // ! aici l-am pus dupa send EOT dar vad ca merge bine asa
         end;
  end;
End;

procedure TCommThread.SendOrderAll;
var ASTMRec : TASTMSegment;
    myPatient : TPatientResult;
    ord_string, cod_analiza , analisys_str: AnsiString;
    i : integer;
    cell_no : AnsiString;
Begin
  // trimite toate order puse in job queue
  OutputDebugMessage('send order all');
  while SendOrd_queue.Count > 0 do begin

     ASTMUnit.ASTM_prevSeqNo := 0;
     ASTMRec := TASTMHeaderSegment.Create;
     (ASTMRec as TASTMHeaderSegment).PacketID := 1;
     (ASTMRec as TASTMHeaderSegment).SegmentName := 'H';
     (ASTMRec as TASTMHeaderSegment).Delimiter := '\^&';
     (ASTMRec as TASTMHeaderSegment).SenderName := SenderName;
     (ASTMRec as TASTMHeaderSegment).ReceiverID := '';//ReceiverName;
     (ASTMRec as TASTMHeaderSegment).ASTMVer := 'E1394-97';
     (ASTMRec as TASTMHeaderSegment).DateAndTime := FormatDateTime('yyyymmddhhnnss', Now);
     (ASTMRec as TASTMHeaderSegment).Processing := 'P';
     (ASTMRec as TASTMHeaderSegment).CommentSI := '';//'TSDWN^REPLY';
     SendStr_queue.Add(ASTMRec.GetASTMSegment);
     SendOrdSeg_queue.Add(ASTMRec);

     // trimit inbuffer de trimis segmente care vor fi impachetate la mom. trimiterii, deci nu direct in string
     //ASTMRec.Free;

//     ASTMRec.Free;
     // send order info
      myPatient := (SendOrd_queue[0] as TPatientResult);

        if myPatient.PatientID = '' then myPatient.PatientID := '1';
         // send patient info
         ASTMRec := TASTMPIDSegment.Create;
         (ASTMRec as TASTMPIDSegment).SegmentName := 'P';
         (ASTMRec as TASTMPIDSegment).SequenceNo := '1';
         (ASTMRec as TASTMPIDSegment).PracticePatID := '';//;
         (ASTMRec as TASTMPIDSegment).LabPatID := myPatient.PatientID;
         (ASTMRec as TASTMPIDSegment).PatID3 := '';
         if (Pos(' ', myPatient.PatientName)>0) then myPatient.PatientName[Pos(' ', myPatient.PatientName)]:='^';
         (ASTMRec as TASTMPIDSegment).Name := myPatient.PatientName;

         if (myPatient.Sex<>'') then
           (ASTMRec as TASTMPIDSegment).Sex := myPatient.Sex[1]
         else
           (ASTMRec as TASTMPIDSegment).Sex := 'M';

         (ASTMRec as TASTMPIDSegment).BirthDate := '';//myPatient.BirthDate;
         (ASTMRec as TASTMPIDSegment).AttPhisician := '';//
         SendStr_queue.Add(ASTMRec.GetASTMSegment);
         SendOrdSeg_queue.Add(ASTMRec);


         analisys_str := '';
     for i := 0 to myPatient.AnalisysNames.Count-1 do begin
        cod_analiza := myPatient.AnalisysNames[i];
        if Length(cod_analiza) = 1 then cod_analiza := '0' + cod_analiza;
        ord_string := '^^^'+cod_analiza;
        if analisys_str<>'' then analisys_str:=analisys_str+'\';
        analisys_str := analisys_str + ord_string;
     end;




         ASTMRec := TASTMTORSegment.Create;
            (ASTMRec as TASTMTORSegment).SegmentName := 'O';
            (ASTMRec as TASTMTORSegment).SequenceNo := '1';
      //      (ASTMrec as TASTMTORSegment).s := myPatient.PatientID;
            (ASTMrec as TASTMTORSegment).SampleID := myPatient.PatientID;// + '^0.0^0^0'; !!! MAYBE FOR ABXPENTRA400 1000 !?
            (ASTMrec as TASTMTORSegment).InstrSpecimenID := '';   //<Sample No>^<Rack ID>^<Position No>^^<Rack Type>^<Container Type>
            (ASTMrec as TASTMTORSegment).UniversalTestID := analisys_str;
            (ASTMrec as TASTMTORSegment).Priority := 'R';
            (ASTMrec as TASTMTORSegment).SpecimenType := '1';
            (ASTMrec as TASTMTORSegment).SpecimentCollectDateTime := FormatDateTime('yyyymmddhhnnss', Now);
            (ASTMrec as TASTMTORSegment).ActionCode := 'A';
            (ASTMrec as TASTMTORSegment).SpecimenType := '1';//serum plasma
            (ASTMrec as TASTMTORSegment).DateTimeResRep := FormatDateTime('yyyymmddhhnnss', Now);

            if true {myPatient.is_batch {}then
                  (ASTMrec as TASTMTORSegment).RecordType := 'O'
            else
                  (ASTMrec as TASTMTORSegment).RecordType := 'Q';

            SendStr_queue.Add(ASTMRec.GetASTMSegment);
            if Assigned(FOrderEntryConfirmation) then
                  FOrderEntryConfirmation(Self, myPatient);

           SendOrdSeg_queue.Add(ASTMRec);
      //     ASTMRec.Free;

            // free patient if come from real time prepare
            if not myPatient.is_batch then begin
      //            OutputDebugMessage('warning: untested astm entry confirmation');
      //            myPatient.Free;       // finally, a calatorit mult
            end;


    // end; {* de data asta nu trebuie *} de la FOR-ul de la anlize



     ASTMRec := TASTMMTRSegment.Create;
     ASTMRec.SegmentName := 'L';
     (ASTMRec as TASTMMTRSegment).SequenceNo := '1';
     (ASTMRec as TASTMMTRSegment).TermCode := 'N';
     SendStr_queue.Add(ASTMRec.GetASTMSegment);
     SendOrdSeg_queue.Add(ASTMRec);
//     ASTMRec.Free;

     SendOrd_Queue.Delete(0);
  end;
End;

procedure TCommThread.ReceiveOrderRequestAll;
var ASTMRec : TASTMSegment;
    sample_id : AnsiString;
    cell_no : AnsiString;
    str_list : TStringList;
    tmp_patient_id, tmp_rack_no, tmp_cell_no : AnsiString;
    PatOrder : TPatientResult;
Begin
  while OrdReq_queue.Count > 0 do begin
    ASTMRec := OrdReq_queue[0] as TASTMSegment;
    sample_id := (ASTMRec as TASTMRIRSegment).StartingRangeID;

    tmp_patient_id := ''; tmp_rack_no := ''; tmp_cell_no := '';

    str_list := TStringList.Create;
    if sample_id[1] = '^' then sample_id:='1'+sample_id;//hack to get 2 lines at least :D

    SplitString( '^',sample_id,str_list);
    if str_list.Count >= 2 then
      tmp_patient_id := trim(str_list[1]);

    str_list.Free;

    cell_no := 'cate una';

    if cell_no = 'ALL' then begin
        if Assigned(FPrepareOrderInformationAll) then
                FPrepareOrderInformationAll(Self, SendOrd_queue, '0');    // method not used
    end
    else begin
      if cell_no <> '' then begin
         PatOrder := TPatientResult.Create;
         try
         // not used in elecsys
           PatOrder.PatientID := tmp_patient_id;

           if Assigned(OnPrepareOrderInformation) then
              OnPrepareOrderInformation(Self, PatOrder, SendOrd_queue);

         // clean up
         finally
           FreeAndNil(PatOrder);
         end;
      end;
    end;

    OrdReq_queue.Delete(0);
  end;
End;

procedure TCommThread.SaveResultsAll;
var ASTMRec, ASTMRec2 : TASTMSegment;
    myPatient : TABXPentra400PatientResult;
    date_hour : AnsiString;
    test_name, test_value : AnsiString;
    str_list : TStringList;
Begin
  str_list := TStringList.Create;
  try
  OutputDebugMessage('saveresultsall:' + inttostr(Res_queue.Count));
  myPatient := TABXPentra400PatientResult.Create;
  if not FCurrentPatient.is_qc
  then begin
      // patient id luat din receive segment
      myPatient.PatientID := (FCurrentPatient.PatientID);
      myPatient.PatientName := FCurrentPatient.PatientName;
  end
  else begin
      myPatient.QCFileInfo := (FCurrentPatient.QCFileInfo);
      myPatient.QCLotInfo := FCurrentPatient.QCLotInfo;
      OutputDebugMessage('save results: control');
  end;

  while Res_queue.Count > 0 do begin
    ASTMRec := Res_queue[0] as TASTMResRecSegment;
//    ASTMRec2 := ResOrd_queue.Pop as TASTMTORSegment;      // goleste resord la un moment dat

    str_list.Clear;
    str_list.Delimiter := '^';
    str_list.DelimitedText := (ASTMRec as TASTMResRecSegment).UniversalTestID;

    //test_name := Trim(StringReplace((ASTMRec as TASTMResRecSegment).UniversalTestID, '^', '', [rfReplaceAll]));

    //test_name := (ASTMRec as TASTMResRecSegment).UniversalTestID;
    if str_list.Count > 4 then
      test_name := str_list[3];
    if Pos('/',test_name) > 0  then test_name := Copy(test_name, 1, Pos('/',test_name)-1);

    test_value := (ASTMRec as TASTMResRecSegment).DataValue;
    myPatient.AnalisysNames.Add(test_name);
    myPatient.AnalisysResults.Add(test_value);

    date_hour := (ASTMRec as TASTMResRecSegment).DateTimeTestCompl;
    myPatient.ResultDate := copy(date_hour, 1, 4) + '-' + copy(date_hour, 5, 2) + '-' + copy(date_hour, 7, 2);
    myPatient.ResultTime := copy(date_hour, 9, 2) + ':' + copy(date_hour, 11, 2);

    Res_queue.Delete(0);
  end;

  // send result to database
  StoreDecodedData(myPatient);

  str_list.Free;
  //FreeAndNil(FCurrentPatient);

  except on E:Exception do
    OutputDebugMessage('Eroare SaveResultsAll:' + E.Message);
  end;
End;

procedure TCommThread.AddOrderEntry(PatientOrder:TPatientResult);
Begin
  SendOrd_queue.Add(PatientOrder);
  // start astm order
  SendOrderAll;
  if SendStr_queue.Count > 0 then begin
    SendString(ENQ);
    ASTMUnit.ASTM_prevSeqNo := 0;
    OutputDebugMessage('sendstr_queue ENQ');
  end;
End;

procedure TCommThread.AddOrderEntryBatchList(PatientsList : TObjectList);
Begin
  // replace buffer
  FreeAndNil(SendOrd_queue);
  SendOrd_queue := PatientsList;
  // create astm messages
  SendOrderAll;
  // start sending astm messages
  if SendStr_queue.Count > 0 then begin
      SendString(ENQ);
      ASTMUnit.ASTM_prevSeqNo := 0;
      OutputDebugMessage('sendstr_queue ENQ');
  end;
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
var ASTMRec : TASTMSegment;
Begin
Exit;
     ASTMUnit.ASTM_prevSeqNo := 0;
     {
     ASTMRec := TASTMHeaderSegment.Create;
     (ASTMRec as TASTMHeaderSegment).PacketID := 1;
     (ASTMRec as TASTMHeaderSegment).SegmentName := 'H';
     (ASTMRec as TASTMHeaderSegment).Delimiter := '\^&';
     (ASTMRec as TASTMHeaderSegment).SenderName := SenderName;
     (ASTMRec as TASTMHeaderSegment).ASTMVer := '1';
     (ASTMRec as TASTMHeaderSegment).DateAndTime := '';//FormatDateTime('yyyymmddhhnnss', Now);
//     (ASTMRec as TASTMHeaderSegment).Processing := 'P';
     SendStr_queue.Add(ASTMRec.GetASTMSegment);
     ASTMRec.Free;


     ASTMRec := TASTMMTRSegment.Create;
     ASTMRec.SegmentName := 'L';
     (ASTMRec as TASTMMTRSegment).SequenceNo := '1';
     SendStr_queue.Add(ASTMRec.GetASTMSegment);
     ASTMRec.Free;
                       {}
     if SendOrdSeg_queue.Count > 0 then begin
       OutputDebugMessage('start beacon:'+ ENQ);
       SendString(ENQ);
       ASTMUnit.ASTM_prevSeqNo := 0;
     end;
End;

procedure TCommThread.timTimeOutTimer(Sender:TObject);
var ASTMRec : TASTMSegment;
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


{** ABXPentra400_ASTM **}
constructor TABXPentra400.Create(AOwner: TComponent);
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

destructor TABXPentra400.Destroy;
begin
  FCommThread.Terminate;
  FCommThread.Free;
  CommBuffer.Free;
  TempBuffer.Free;
  inherited;
end;

procedure TABXPentra400.SetOnDataArrived(AProc:TOnDataArrived);
Begin
  inherited;
  FCommThread.FOnDataArrived := AProc;
End;

procedure TABXPentra400.SetOnOutputDebugMessage(AProc:TOutputDebugMessage);
Begin
  inherited;
  FCommThread.OnOutputDebugMessage := AProc;
End;

procedure TABXPentra400.SetPrepareOrderInformation(AProc:TPrepareOrderInformation);
Begin
  inherited;
  FCommThread.OnPrepareOrderInformation := AProc;
End;

procedure TABXPentra400.SetPrepareOrderInformationAll(AProc:TPrepareOrderInformationAll);
Begin
  inherited;
  FCommThread.OnPrepareOrderInformationAll := AProc;
End;

procedure TABXPentra400.SetOrderEntryConfirmation(AProc:TOrderEntryConfirmation);
Begin
  inherited;
  FCommThread.OnOrderEntryConfirmation := AProc;
End;

procedure TABXPentra400.DoActive(Value: Boolean);
begin
  FActive := Value;
  FCommThread.Active := FActive;
end;

procedure TABXPentra400.SetNoHandShake(Value: Boolean);
begin
  if FNoHandShake = Value then Exit;
  FNoHandShake := Value;
  FCommThread.NoHandShake := FNoHandShake;
end;

procedure TABXPentra400.CommTimerTimer(Sender:TObject);
{* citeste de pe comm buffer si trimite la DB *}
var DataObject : TABXPentra400PatientResult;
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
    DataObject := TempBuffer[0] as TABXPentra400PatientResult;
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



function TABXPentra400.AddOrderEntry(PatientOrder:TPatientResult):boolean;
Begin
  FCommThread.AddOrderEntry(PatientOrder);
End;

function TABXPentra400.AddOrderEntryBatchList(PatientsList : TObjectList):boolean;
Begin
  FCommThread.AddOrderEntryBatchList(PatientsList);
End;




procedure TABXPentra400.Test;
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

  //FCommThread.TestReceive(#2'1H|\^&|||abxpentra400^1|||||host|TSREQ^REAL|P|1'#$D'Q|1|^^                449169^0^5002^2^^S1^SC||ALL||||||||O'#$D'L|1|N'#$D#3'B1'#$D#$A);
  //FCommThread.TestReceive(EOT);
                 {}

                 FCommThread.SendString(ENQ);
                 Exit;
FCommThread.TestReceive(#2'1H|\^&|||ABXP400|||||||P|E1394-97|20141001142141'#3'2D');
FCommThread.TestReceive(#2'2P|1||7||IVAN^ALEXANDRU|||M'#3'57');
FCommThread.TestReceive(#2'3O|1|7||||20141001135613|20141001134400||||||||1||||||||||F'#3'FE');
FCommThread.TestReceive(#2'4R|1|^^^211^CREA3|0.38|15||L||F|||20141001135905'#3'F6');
FCommThread.TestReceive(#2'5C|1|I|Flag^NORM_RANGEL^CALC_ERROR|I'#3'C1');
FCommThread.TestReceive(#2'6R|2|^^^5^AST|42|26||H||F|||20141001135817'#3'D1');
FCommThread.TestReceive(#2'7C|1|I|Flag^NORM_RANGEH^CALC_ERROR|I'#3'BF');
FCommThread.TestReceive(#2'0R|3|^^^25^GluP|89.91|15||||F|||20141001135929'#3'EB');
FCommThread.TestReceive(#2'1R|4|^^^39^Urea|12.27|15||L||F|||20141001135829'#3'43');
FCommThread.TestReceive(#2'2C|1|I|Flag^NORM_RANGEL^CALC_ERROR|I'#3'BE');
FCommThread.TestReceive(#2'3R|5|^^^998^COL_R|94.5|15||A||F|||20141001135805'#3'4B');
FCommThread.TestReceive(#2'4C|1|I|Flag^LINEARITY_HIGH^NORM_RANGEL^CALC_ERROR|I'#3'4E');
FCommThread.TestReceive(#2'5R|6|^^^997^TRIGL_R|119|15||A||F|||20141001140429'#3'BA');
FCommThread.TestReceive(#2'6C|1|I|Flag^LINEARITY_HIGH^CALC_ERROR|I'#3'9E');
FCommThread.TestReceive(#2'7L|1|N'#3'0A');

End;



end.
