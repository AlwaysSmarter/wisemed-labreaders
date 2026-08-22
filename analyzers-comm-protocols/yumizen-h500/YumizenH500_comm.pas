	unit YumizenH500_comm;
// threading -ok
//


interface

uses SysUtils, Classes, Types, Windows, SyncObjs, ExtCtrls, StrUtils, SerialNG, Dialogs,
  ScktComp, Contnrs, DateUtils, forms, Graphics, ASTMUnit,
  u_CIFCommObj, u_CIFUtils   ;

const
  FullASTM = True;                    // ASTM fields padded with ^
  Hemato_names : array[1..26] of String =        ('WBC','LYM#','LYM%','MON#','MON%','NEU#','NEU%','EOS#','EOS%','BAS#','BAS%','RBC','Hgb','Hct','MCV','MCH','MCHC','RDW','PLT','MPV', 'THT', 'PDW','ALY#', 'ALY%','LIC#', 'LIC%');
  Hemato_names_codes : array[1..26] of String =  (  '!',   '"',    '#',  '$',   '%',   '(',   ')',   '*',   '+',   ',',   '-',  '2',  '3',  '4',  '5',  '6',   '7',  '8',  '@',  'A',   'B',   'C',   '.',    '/',   '0',    '1');


type
  ECEInvalidHeader = Exception;

  TYumizenH500PatientResult = class (TPatientResult)

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
    procedure ParseClusterCOM(Data:AnsiString);
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

  TYumizenH500 = class(TCIFCommObj)
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
  CR = #13;
  SOH = #1;



implementation
uses StringUtils;

function OnlyNumbers(str : AnsiString):AnsiString;
var Idx : Integer;
    tmp_str : AnsiString;
begin
  Idx := 1;
  tmp_str := '';
  while Idx <= Length(str) do
    begin
       if (ord(str[Idx]) > 47) and (ord(str[Idx]) < 58) then
         tmp_str := tmp_str + str[Idx];
       if (str[Idx] = '.') then tmp_str := tmp_str + str[Idx];
       Inc(idx);
    end;
  Result := tmp_str;
end;

procedure SplitString(SourceStr:AnsiString; Delimiter:AnsiChar; DestList:TStringList);
var i : integer;
    temp_str : AnsiString;
Begin
  temp_str := '';
  DestList.Clear;
  for i := 1 to length(SourceStr) do
    if SourceStr[i] <> Delimiter then
        temp_str := temp_str + SourceStr[i]
    else begin
        DestList.Add(temp_str);
        temp_str := '';
    end;
  if temp_str <> '' then DestList.Add(temp_str);
End;


{=============== Communication Layer ==========================================}

// ** Communication Thread ** //
constructor TCommThread.Create(AOwner : TObject; ABuffer:TObjectList; ACS:TCriticalSection);
Begin
  inherited Create(True);
  FreeOnTerminate := False;     // f. important
  OnTerminate := ThreadTerminate;
  Owner := AOwner;

  if (param_str_has_option('TCPIP')) then
    begin
      Sock := TServerSocket.Create(nil);
      Sock.Port := 5150;
      Sock.ServerType := stNonBlocking;
      sock.OnClientRead := SocketRead;
      sock.OnAccept := SocketConnect;
      Comm := nil;
    end
  else
    begin
      Comm := TSerialPortNG.Create(nil);
      with Comm do
        begin
          BaudRate := 19200;
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
    end;
  //

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
  OutputDebugMessage('set active');
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

function remove_leading_zeros(txt:AnsiString):AnsiString;
var i:integer;
begin
  i:=1;
  while (length(txt)>0) and (txt[i]='0') do
    begin
      i:=1;
      txt:=copy(txt, 2, Length(txt)-1);
    end;
  Result:=txt;
end;

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
var s: AnsiString;
Begin
  CS.Acquire;
  SetLength(s, Socket.ReceiveLength);
  Socket.ReceiveBuf(Pointer(s)^, Length(s));
  ParseCluster(s);
  CS.Release;
  Synchronize(DoDataArrived);
End;

procedure TCommThread.CommRxClusterEvent(Sender: TObject);
begin
  if Comm.NextClusterSize > 0 then
    begin
      CS.Acquire;
      ParseClusterCOM(Comm.ReadNextClusterAsString);
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
  (Owner as TYumizenH500).IsCommunication := FIsCommunication;
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
    idx_SegName : integer;
    wsData : string;      // widestring
Begin
    Result := False;
    idx_SegName := 1;
    ASTMRec := nil;
    OutputDebugMessage('Parse Record:' + Data[idx_SegName]);
    try
    case Data[idx_SegName] of
    'H' : begin  // header
            ASTMRec := TASTMHeaderSegment.Create;
    end;
    'P' : begin  // Patient
            ASTMRec := TASTMPIDSegment.Create;
    end;
    'O' : begin  // order  - asta e fapt il trimit eu
            ASTMRec := TASTMTORSegment.Create;
    end;
    'Q' : begin  // order request
            ASTMRec := TASTMRIRSegment.Create;
    end;
    'R' : begin  // result
            ASTMRec := TASTMResRecSegment.Create;
    end;
    'L' : begin  // termination
            ASTMRec := TASTMMTRSegment.Create;
    end;
    'M' : begin  // termination
            ASTMRec := TASTMMIRSegment.Create;
    end;
    end;

    if ASTMRec <> nil then begin
        wsData := String(Data);
        ASTMRec.ParseASTMSegmentFromString(wsData);
        Result := True;
        ReceiveSegment(ASTMRec);
    end
    else
        Result := True;
    except // eat exception -> result = false;
        on E:Exception do OutputDebugMessage('Eroare neidentificata:'+E.Message);
    end;
End;

procedure TCommThread.ReceiveSegment(ASTMRec:TASTMSegment);
var str_list : TStringList;
Begin
  OutputDebugMessage('received segment:' + ASTMRec.SegmentName[1]);
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
            // push order request in job queue
            OrdReq_queue.Add(ASTMRec);
        end;
  'O' : begin
            // store order information (fileID) - ResOrd_q si Res_q merg in paralel (se asociaza dupa index in lista)
            OutputDebugMessage('SegReceived order');

            if FCurrentPatient <> nil then
                  if FCurrentPatient.PatientID = '' then begin
                      // not used on elecsys
                      str_list := TStringList.Create;
                      SplitString((ASTMRec as TASTMTORSegment).SampleID, '^', str_list);
                      if str_list.Count >= 1 then
                        FCurrentPatient.PatientID := str_list[0];
                      FCurrentPatient.PatientID := remove_leading_zeros(trim(FCurrentPatient.PatientID));
//                       !!!!
                      if str_list.Count >= 5 then begin
                            FCurrentPatient.is_qc := (Pos('CONTROL', str_list[4]) > 0);
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
  if (Data='') then exit;
  
  OutputDebugMessage(Format('===> %s',[Data]));

  if Assigned(FOnDataArrived) then
        FOnDataArrived(Self);{}

  f_DataArrived := True;

  // reset timeout timer
  FtimTimeOut.Enabled := False;
  FtimTimeOut.Enabled := True;
  CheckTimeOut := False;
  IsCommunication := True;

  //ParseRecord(Data);            // numai pt testare

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
    ASTMUnit.ASTM_prevSeqNo := 0;
    SendString(ACK);
    SetInitialized(True);       // check initialized on start communication
  end;

  STX : begin
          Data := Copy(Data, 3, Length(Data)-7);  // am scos STX si CR ETX C1 C2 CR LF
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
{
              if SendStr_queue.Count > 0 then begin
                    OutputDebugMessage('sendsstr_queue: ' + SendStr_queue[0]);

                    SendString(SendStr_queue[0]);
                    SendStr_queue.Delete(0);
              end
              else SendString(EOT);
 {}
               if SendOrdSeg_queue.Count > 0 then begin
                    SendString((SendOrdSeg_queue[0] as TASTMSegment).GetASTMSegment);
                    SendOrdSeg_queue.Delete(0);
              end
              else SendString(EOT);

        end;
  NAK : Begin
            //showmessage('Eroare de comunicatie - NAK !');
            OutputDebugMessage('Eroare de comunicatie - NAK !');
            SendStr_queue.Clear;
            SendOrdSeg_queue.Clear;
  end;

  EOT : begin   // raspund la request-ul lui (Q )
            SetInitialized(False);
            // goleste job queue
            OutputDebugMessage('EOT');
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

            end
            else
                SendString(ACK);    // ! aici l-am pus dupa send EOT dar vad ca merge bine asa
         end;
  end;
End;

procedure TCommThread.ParseClusterCOM(Data: AnsiString);
  procedure CheckBuffer(check_string: AnsiString);
  var
    Idx, StartID, i,j: Integer;
    tmpRes : AnsiString;
    rezultat_efectiv : AnsiString;
    char_id : char;
    current_string : AnsiString;
    machineRecvdData : TStringList;
    CurrentPatient: TPatientResult;
  begin
       while (AnsiPos(STX, check_string)<>0) and (AnsiPos(ETX, check_string)<>0) do
       begin
          current_string := Copy(check_string,AnsiPos(STX, check_string)+1, AnsiPos(ETX, check_string)-AnsiPos(STX, check_string));
          check_string :=  Copy(check_string, AnsiPos(ETX, check_string)+1, Length(check_string));

            CurrentPatient := TPatientResult.Create;

            machineRecvdData := TStringList.Create;
            try
              SplitString(current_string, CR, machineRecvdData);
              for Idx := 0 to machineRecvdData.Count - 1 do
                begin
                   if (machineRecvdData[Idx][1] = 'q') then
                     begin
                       rezultat_efectiv := Trim(Copy(machineRecvdData[Idx], 2, Length(machineRecvdData[Idx])));
                       if (Pos(' ', rezultat_efectiv) > 0) then
                         rezultat_efectiv := Copy(rezultat_efectiv, 0, Pos(' ', rezultat_efectiv));
                       CurrentPatient.ResultDate := '20' + copy(rezultat_efectiv,7,2) + '-' + copy(rezultat_efectiv,4,2) + '-' + copy(rezultat_efectiv,0,2);
                       CurrentPatient.ResultTime := copy(rezultat_efectiv,10,2) + ':' + copy(rezultat_efectiv,13,2);
                       OutputDebugMessage(Format('Result date time:  %s',[rezultat_efectiv]));
                     end
                   else
                   if (machineRecvdData[Idx][1] = 'u') then
                     begin
                       rezultat_efectiv := Trim(Copy(machineRecvdData[Idx], 2, Length(machineRecvdData[Idx])));
                       if (Pos(' ', rezultat_efectiv) > 0) then
                         rezultat_efectiv := Copy(rezultat_efectiv, 0, Pos(' ', rezultat_efectiv));
                       CurrentPatient.PatientID := rezultat_efectiv;
                     end
                   else
                   if (machineRecvdData[Idx][1] = 'v') then
                     begin
                       rezultat_efectiv := Trim(Copy(machineRecvdData[Idx], 2, Length(machineRecvdData[Idx])));
                       if (Pos(' ', rezultat_efectiv) > 0) then
                         rezultat_efectiv := Copy(rezultat_efectiv, 0, Pos(' ', rezultat_efectiv));
                       CurrentPatient.PatientName := rezultat_efectiv;
                       if CurrentPatient.PatientID = '' then CurrentPatient.PatientID := CurrentPatient.PatientName;

                     end
                   else
                   if (machineRecvdData[Idx][1] = 'W') then
                     begin
                       rezultat_efectiv := Copy(machineRecvdData[Idx], 2, Length(machineRecvdData[Idx]));
                       rezultat_efectiv := Trim(rezultat_efectiv);
                       for I := 1 to Length(rezultat_efectiv) do
                         CurrentPatient.WBCGraph.Add(IntToStr(Ord(rezultat_efectiv[i])));
                       //for I := Length(rezultat_efectiv)+1 to 128 do
                       //  CurrentPatient.WBCGraph.Add('0');
                     end
                   else
                   if (machineRecvdData[Idx][1] = 'X') then
                     begin
                       rezultat_efectiv := Copy(machineRecvdData[Idx], 2, Length(machineRecvdData[Idx]));
                       rezultat_efectiv := Trim(rezultat_efectiv);
                       for I := 1 to Length(rezultat_efectiv) do
                         CurrentPatient.RBCGraph.Add(IntToStr(Ord(rezultat_efectiv[i])));
                       //for I := Length(rezultat_efectiv)+1 to 128 do
                       //  CurrentPatient.RBCGraph.Add('0');
                     end
                   else
                   if (machineRecvdData[Idx][1] = 'Y') then
                     begin
                       rezultat_efectiv := Copy(machineRecvdData[Idx], 2, Length(machineRecvdData[Idx]));
                       rezultat_efectiv := Trim(rezultat_efectiv);
                       for I := 1 to Length(rezultat_efectiv) do
                         CurrentPatient.PLTGraph.Add(IntToStr(Ord(rezultat_efectiv[i])));
                      // for I := Length(rezultat_efectiv)+1 to 128 do
                      //   CurrentPatient.PLTGraph.Add('0');
                     end
                   else
                     begin
                     //parse only the data I know here
                       for j:=1 to 26 do
                         if (Hemato_names_codes[j] = machineRecvdData[Idx][1]) then
                           begin
                             rezultat_efectiv := Trim(Copy(machineRecvdData[Idx], 2, Length(machineRecvdData[Idx])));
                             if (Pos(' ', rezultat_efectiv) > 0) then
                               rezultat_efectiv := Copy(rezultat_efectiv, 0, Pos(' ', rezultat_efectiv));
                             rezultat_efectiv := remove_leading_zeros(rezultat_efectiv);
                             rezultat_efectiv := onlyNumbers(rezultat_efectiv);
                             if (rezultat_efectiv <> '') then
                               if (rezultat_efectiv[1]='.') then rezultat_efectiv:='0'+rezultat_efectiv;
                             CurrentPatient.AnalisysNames.Add(Hemato_names[j]);
                             CurrentPatient.AnalisysResults.Add(rezultat_efectiv);
                             break;
                           end;
                     end
                end;
              StoreDecodedData(CurrentPatient);
            finally
               machineRecvdData.Free;
               current_string:='';
           end;
       end;
  end;
var i:integer;
    check_string : AnsiString;
begin
//De fiecare data cand mai primesc un cluster -> ma uit sa vad daca am primit
//char de inceput STX - 02h si char de sfarsit : ETX - 03h
  OutputDebugMessage(Format('>>> %s',[Data]));
  OutputDebugMessage(Format('H>>> %s',[StrToHex(Data)]));
  FBufferText := Format('%s%s', [FBufferText, Data]); //non sense to load the buffer as long as I already have EndTag

  while (AnsiPos(SOH,FBufferText)<>0) and (AnsiPos(EOT, FBufferText)<>0) do
  begin
    check_string := Copy(FBufferText, AnsiPos(SOH,FBufferText)+1, AnsiPos(EOT,FBufferText)-AnsiPos(SOH,FBufferText));
    FBufferText := Copy(FBufferText, AnsiPos(EOT,FBufferText)+1, Length(FBufferText));
    CheckBuffer(check_string); //never in buffer more than a package - too lasy sender
  end;
end;

procedure TCommThread.SendOrderAll;
var ASTMRec : TASTMSegment;
    myPatient : TPatientResult;
    ord_string, cod_analiza : AnsiString;
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
     (ASTMRec as TASTMHeaderSegment).SenderName := 'HOST';
     (ASTMRec as TASTMHeaderSegment).ASTMVer := 'E1394-97';
     (ASTMRec as TASTMHeaderSegment).DateAndTime := FormatDateTime('yyyymmddhhnnss', Now);
     (ASTMRec as TASTMHeaderSegment).Processing := 'P';
     SendStr_queue.Add(ASTMRec.GetASTMSegment);
     SendOrdSeg_queue.Add(ASTMRec);

     // trimit inbuffer de trimis segmente care vor fi impachetate la mom. trimiterii, deci nu direct in string
     //ASTMRec.Free;

     myPatient := (SendOrd_queue[0] as TPatientResult);
     if myPatient.PatientID = '' then myPatient.PatientID := '1';
     // send patient info
     ASTMRec := TASTMPIDSegment.Create;
     (ASTMRec as TASTMPIDSegment).SegmentName := 'P';
     (ASTMRec as TASTMPIDSegment).SequenceNo := '1';
     (ASTMRec as TASTMPIDSegment).PracticePatID := '';
     (ASTMRec as TASTMPIDSegment).LabPatID := myPatient.PatientID;
     (ASTMRec as TASTMPIDSegment).Name := '';
     (ASTMRec as TASTMPIDSegment).Sex := '';
     (ASTMRec as TASTMPIDSegment).BirthDate := '';
     SendStr_queue.Add(ASTMRec.GetASTMSegment);
     SendOrdSeg_queue.Add(ASTMRec);
//     ASTMRec.Free;
     // send order info

     ord_string := '';
     for i := 0 to myPatient.AnalisysNames.Count-1 do begin
        cod_analiza := myPatient.AnalisysNames[i];
        if Length(cod_analiza) = 1 then cod_analiza := '0' + cod_analiza;
        if not FullASTM then begin
            if ord_string <> '' then ord_string := ord_string + '^';
            ord_string := ord_string + cod_analiza;
        end
        else begin
            if ord_string <> '' then ord_string := ord_string + '\';
            ord_string := ord_string + '^^^' + cod_analiza + '^';
        end;
     end; {* de data asta nu trebuie *}

      ASTMRec := TASTMTORSegment.Create;
      (ASTMRec as TASTMTORSegment).SegmentName := 'O';
      (ASTMRec as TASTMTORSegment).SequenceNo := '1';
//      (ASTMrec as TASTMTORSegment).s := myPatient.PatientID;
      (ASTMrec as TASTMTORSegment).SampleID := myPatient.PatientID + '^0.0^0^0';
      (ASTMrec as TASTMTORSegment).UniversalTestID := ord_string;
      (ASTMrec as TASTMTORSegment).Priority := 'R';
      (ASTMrec as TASTMTORSegment).SpecimenType := '1';
//      (ASTMrec as TASTMTORSegment).RequestedDateTime := FormatDateTime('yyyymmddhhnnss', Now);
//      (ASTMrec as TASTMTORSegment).ActionCode := 'N';
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
    str_list.Delimiter := '^';
    str_list.DelimitedText := sample_id;
    if str_list.Count >= 2 then
      tmp_patient_id := str_list[1];

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
    myPatient : TYumizenH500PatientResult;
    date_hour : AnsiString;
    test_name, test_value : AnsiString;
    str_list : TStringList;
Begin
  str_list := TStringList.Create;
  try
  OutputDebugMessage('saveresultsall:' + inttostr(Res_queue.Count));
  myPatient := TYumizenH500PatientResult.Create;
  if not FCurrentPatient.is_qc
  then begin
      // patient id luat din receive segment
      myPatient.PatientID := Trim(remove_leading_zeros((FCurrentPatient.PatientID)));
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
    SplitString((ASTMRec as TASTMResRecSegment).UniversalTestID, '^', str_list);

    test_name := Trim(StringReplace((ASTMRec as TASTMResRecSegment).UniversalTestID, '^', '', [rfReplaceAll]));
    if str_list.Count > 3 then
      test_name := str_list[3];
    test_value := (ASTMRec as TASTMResRecSegment).DataValue;
    myPatient.AnalisysNames.Add(test_name);
    myPatient.AnalisysResults.Add(test_value);
    OutputDebugMessage('>> TN:' + test_name);
    OutputDebugMessage('>> TV:' + test_value);

    date_hour := (ASTMRec as TASTMResRecSegment).DateTimeTestCompl;
    myPatient.ResultDate := copy(date_hour, 1, 4) + '-' + copy(date_hour, 5, 2) + '-' + copy(date_hour, 7, 2);
    myPatient.ResultTime := copy(date_hour, 9, 2) + ':' + copy(date_hour, 11, 2);

    Res_queue.Delete(0);
  end;

  // send result to database
  StoreDecodedData(myPatient);

  str_list.Free;
  FreeAndNil(FCurrentPatient);

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
  ParseClusterCOM(Data);
End;

procedure TCommThread.StartBeacon;
var ASTMRec : TASTMSegment;
Begin
     ASTMUnit.ASTM_prevSeqNo := 0;
     {
     ASTMRec := TASTMHeaderSegment.Create;
     (ASTMRec as TASTMHeaderSegment).PacketID := 1;
     (ASTMRec as TASTMHeaderSegment).SegmentName := 'H';
     (ASTMRec as TASTMHeaderSegment).Delimiter := '\^&';
     (ASTMRec as TASTMHeaderSegment).SenderName := 'HOST';
     (ASTMRec as TASTMHeaderSegment).ASTMVer := 'E1394-97';
     (ASTMRec as TASTMHeaderSegment).DateAndTime := FormatDateTime('yyyymmddhhnnss', Now);
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


{** YumizenH500_ASTM **}
constructor TYumizenH500.Create(AOwner: TComponent);
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

destructor TYumizenH500.Destroy;
begin
  FCommThread.Terminate;
  FCommThread.Free;
  CommBuffer.Free;
  TempBuffer.Free;
  inherited;
end;

procedure TYumizenH500.SetOnDataArrived(AProc:TOnDataArrived);
Begin
  inherited;
  FCommThread.FOnDataArrived := AProc;
End;

procedure TYumizenH500.SetOnOutputDebugMessage(AProc:TOutputDebugMessage);
Begin
  inherited;
  FCommThread.OnOutputDebugMessage := AProc;
End;

procedure TYumizenH500.SetPrepareOrderInformation(AProc:TPrepareOrderInformation);
Begin
  inherited;
  FCommThread.OnPrepareOrderInformation := AProc;
End;

procedure TYumizenH500.SetPrepareOrderInformationAll(AProc:TPrepareOrderInformationAll);
Begin
  inherited;
  FCommThread.OnPrepareOrderInformationAll := AProc;
End;

procedure TYumizenH500.SetOrderEntryConfirmation(AProc:TOrderEntryConfirmation);
Begin
  inherited;
  FCommThread.OnOrderEntryConfirmation := AProc;
End;

procedure TYumizenH500.DoActive(Value: Boolean);
begin
  FActive := Value;
  if Assigned(FCommThread) then FCommThread.Active := FActive;
end;

procedure TYumizenH500.SetNoHandShake(Value: Boolean);
begin
  if FNoHandShake = Value then Exit;
  FNoHandShake := Value;
  FCommThread.NoHandShake := FNoHandShake;
end;

procedure TYumizenH500.CommTimerTimer(Sender:TObject);
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



function TYumizenH500.AddOrderEntry(PatientOrder:TPatientResult):boolean;
Begin
  FCommThread.AddOrderEntry(PatientOrder);
End;

function TYumizenH500.AddOrderEntryBatchList(PatientsList : TObjectList):boolean;
Begin
  FCommThread.AddOrderEntryBatchList(PatientsList);
End;




procedure TYumizenH500.Test;
var
  str_res : TStringList;
  i : integer;

Begin
  str_res := TStringList.Create;
  str_res.LoadFromFile('../DOCS/YUMIZENH500.LOG');

  for i := 0 to str_res.Count-1 do
    fcommthread.TestReceive(str_res[i]+CR);

exit;
fcommthread.TestReceive(ENQ);
 fcommthread.TestReceive(STX+'1H|\^&|||ABX|||||||P|E1394-97|20150603131350'+#13+ETX+'51');
 fcommthread.TestReceive(STX+'2P|1'+#13+ETX+'3F');
 fcommthread.TestReceive(STX+'3O|1|221460^01^01||^^^|||||||||||||||||||||F'+#13+ETX+'EC');
 fcommthread.TestReceive(STX+'4C|1|I|Alarm_ANALYSER^FOR INVESTIGATIONAL USE ONLY|I'+#13+ETX+'2A');
 fcommthread.TestReceive(STX+'5R|1|^^^WBC^804-5^1|7.3|1||||F||ABX ABX'+#13+ETX+'66');
 fcommthread.TestReceive(STX+'6R|2|^^^LYM#^731-0^1|2.66|1||||F||ABX ABX'+#13+ETX+'CF');
 fcommthread.TestReceive(STX+'7R|3|^^^LYM%^736-9^1|36.6|1||||F||ABX ABX'+#13+ETX+'E2');
 fcommthread.TestReceive(STX+'0R|4|^^^MON#^742-7^1|0.52|1||||F||ABX ABX'+#13+ETX+'C5');
 fcommthread.TestReceive(STX+'1R|5|^^^MON%^744-3^1|7.2|1||||F||ABX ABX'+#13+ETX+'99');
 fcommthread.TestReceive(STX+'2R|6|^^^NEU#^751-8^1|3.72|1||||F||ABX ABX'+#13+ETX+'CD');
 fcommthread.TestReceive(STX+'3R|7|^^^NEU%^770-8^1|51.1|1||||F||ABX ABX'+#13+ETX+'CD');
 fcommthread.TestReceive(STX+'4R|8|^^^EOS#^711-2^1|0.32|1||||F||ABX ABX'+#13+ETX+'BF');
 fcommthread.TestReceive(STX+'5R|9|^^^EOS%^713-8^1|4.4|1||||F||ABX ABX'+#13+ETX+'9E');
 fcommthread.TestReceive(STX+'6R|10|^^^BAS#^704-7^1|0.05|1||||F||ABX ABX'+#13+ETX+'E0');
 fcommthread.TestReceive(STX+'7R|11|^^^BAS%^706-2^1|0.7|1||||F||ABX ABX'+#13+ETX+'B3');
 fcommthread.TestReceive(STX+'0R|12|^^^ALY#^733-6^1|0.06|1||||F||ABX ABX'+#13+ETX+'EE');
 fcommthread.TestReceive(STX+'1R|13|^^^ALY%^735-1^1|0.8|1||||F||ABX ABX'+#13+ETX+'C1');
 fcommthread.TestReceive(STX+'2R|14|^^^LIC#^X-LIC^1|0.07|1||||F||ABX ABX'+#13+ETX+'42');
 fcommthread.TestReceive(STX+'3R|15|^^^LIC%^11117-9^1|1.0|1||||F||ABX ABX'+#13+ETX+'14');
 fcommthread.TestReceive(STX+'4R|16|^^^RBC^789-9^1|4.70|1||||F||ABX ABX'+#13+ETX+'D7');
 fcommthread.TestReceive(STX+'5R|17|^^^HGB^717-9^1|11.7|1||||F||ABX ABX'+#13+ETX+'C8');
 fcommthread.TestReceive(STX+'6R|18|^^^HCT^4544-3^1|35.4|1||L||F||ABX ABX'+#13+ETX+'53');
 fcommthread.TestReceive(STX+'7R|19|^^^MCV^787-2^1|75|1||L||F||ABX ABX'+#13+ETX+'D2');
 fcommthread.TestReceive(STX+'0R|20|^^^MCH^785-6^1|24.9|1||L||F||ABX ABX'+#13+ETX+'18');
 fcommthread.TestReceive(STX+'1R|21|^^^MCHC^786-4^1|33.0|1||||F||ABX ABX'+#13+ETX+'07');
 fcommthread.TestReceive(STX+'2R|22|^^^RDW^788-0^1|17.4|1||HH||F||ABX ABX'+#13+ETX+'6F');
 fcommthread.TestReceive(STX+'3R|23|^^^PLT^777-3^1|234|1||||F||ABX ABX'+#13+ETX+'B4');
 fcommthread.TestReceive(STX+'4R|24|^^^MPV^776-5^1|9.2|1||||F||ABX ABX'+#13+ETX+'BA');
 fcommthread.TestReceive(STX+'5R|25|^^^PCT^X-PCT^1|0.216|1||||F||ABX ABX'+#13+ETX+'74');
 fcommthread.TestReceive(STX+'6R|26|^^^PDW^X-PDW^1|16.8|1||||F||ABX ABX'+#13+ETX+'54');
 fcommthread.TestReceive(STX+'7L|1|N'+#13+ETX+'0A'); fcommthread.TestReceive(EOT);
  str_res.Free;
                 {}


  // test beacon
  FCommThread.iscommunication := false;
                 {}
End;



end.
