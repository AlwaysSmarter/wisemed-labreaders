unit ASTMUnit;

interface
uses SysUtils;


Type

  TASTMSegment = class(TObject)
    PacketID : Byte;
    FieldsNo : Byte;
    FieldSeparator : String[1];
    SegmentName : String;
    EncChars : String[4];
    function GetASTMSegment:String;
    procedure ParseASTMSegmentFromString(var ASTMString : String);
    function CheckControl(Data: String): Integer;
    procedure SetASTMSegmentField(Idx:Byte; Val : String);virtual;abstract;
    function GetASTMSegmentField(Idx:Byte): String;virtual;abstract;
  private
  public
  end;

  TASTMHeaderSegment = class(TASTMSegment)
    Delimiter : string[4];
    MessageControlID : String[0];
    AccessPassword : String[0];
    SenderName : String[32];
    SenderStrAddr : String[26];
    ReservedField : String[40];
    SenderPhoneNo : String[7];
    SenderCharacteristics : String[20];
    ReceiverID : String[10];
    CommentSI : String[180];
    Processing : String[180];
    ASTMVer : String[10];
    DateAndTime : String[14];
    constructor Create;
    procedure SetASTMSegmentField(Idx:Byte; Val : String);override;
    function GetASTMSegmentField(Idx:Byte): String;override;

  end;
  TASTMPIDSegment = class(TASTMSegment)
    SequenceNo : String[3];
    PracticePatID : String[15];
    LabPatID : String[80];
    PatID3 : String[80];
    Name : String[36];
    MothersMName : String[36];
    BirthDate : String[8];
    Sex : String[1];
    Race : String[80];
    Address : String[60];
    ResF1 : string[80];
    Phone : String[40];
    AttPhisician : String[80];
    SpecField1 : String[80];
    SpecField2 : String[80];
    PacHeight : String[80];
    PacWeight : String[80];
    KnownDiag : String[80];
    ActiveMed : String[80];
    Diet : String[80];
    PracticeF1 : String[80];
    PracticeF2 : String[80];
    AdmissionDate : String[8];
    AdmissionStat : String[80];
    Location : String[20];
    NatureOfDiag : String[80];
    AltDiagCode : String[80];
    Religion : String[40];
    MaritalStatus : String[40];
    IsolationStatus : String[40];
    Language : String[40];
    HospService : String[40];
    HospInst : String[40];
    DosageCat : String[40];
    constructor Create;
    procedure SetASTMSegmentField(Idx:Byte; Val : String);override;
    function GetASTMSegmentField(Idx:Byte): String;override;
  end;

  TASTMTORSegment = class(TASTMSegment)
    SequenceNo : String[4];
    SampleID : String[15]; //also named SpecimenID on some docs
    InstrSpecimenID : String[80];
    UniversalTestID : String;
    Priority : String[10];
    RequestedDateTime : String[14];
    SpecimentCollectDateTime : String[14];
    CollectionEndTime : String[14];
    CollectionVol : String[60];
    CollectorID : String[60];
    ActionCode: String[60];
    DangerCode: String[60];
    RelevantClInfo: String[60];
    DateTimeSpecRecvd : String[14];
    SpecimenType : String[1];
    OrderingPhysician : String[80];
    PhysiscianPhone : String[80];
    UserFld1 : String[80];
    UserFld2 : String[80];
    LabFld1 : String[80];
    LabFld2 : String[80];
    DateTimeResRep : String[14];
    InstrumentCharge : String[80];
    InstrumentSectionID : String[80];
    RecordType : String[1];
    ResFld : String[80];
    LocOfSpecCol : String[80];
    NosocomialInfFlg : String[80];
    SpecimenService : String[80];
    SpecimenInst : String[80];
    constructor Create;
    procedure SetASTMSegmentField(Idx:Byte; Val : String);override;
    function GetASTMSegmentField(Idx:Byte): String;override;
  end;

  TASTMResRecSegment = class(TASTMSegment)
    SequenceNo : String[3];
    UniversalTestID : String;
    DataValue : String;
    UnitsOfMeas : String[6];
    RefRang : String[21];
    Flags : String[42];
    NatureOfAbn : String[60];
    ResStatus : String[1];
    DateOfChange: String[60];
    OperatorID: String[60];
    DateTimeTestStarted: String[14];
    DateTimeTestCompl : String[14];
    InstrumentID : String[1];
    constructor Create;
    procedure SetASTMSegmentField(Idx:Byte; Val : String);override;
    function GetASTMSegmentField(Idx:Byte): String;override;
  end;

  TASTMMTRSegment = class(TASTMSegment)
    SequenceNo : String[1];
    TermCode : String[1];
    constructor Create;
    procedure SetASTMSegmentField(Idx:Byte; Val : String);override;
    function GetASTMSegmentField(Idx:Byte): String;override;
  end;

  TASTMRIRSegment = class(TASTMSegment)
    SequenceNo : String[1];
    StartingRangeID : String[31];
    EndingRangeID : String[31];
    UniversalTestId : String;
    NatureReqTL : String[80];
    BegReqResDateTime : String[14];
    EndReqResDateTime : String[14];
    ReqPhysicianName : String[80];
    ReqPhysicianPhone : String[80];
    UserFld1 : String[80];
    UserFld2 : String[80];
    ReqInfoStatus : String[1];
    constructor Create;
    procedure SetASTMSegmentField(Idx:Byte; Val : String);override;
    function GetASTMSegmentField(Idx:Byte): String;override;
  end;

  TASTMMIRSegment = class(TASTMSegment)     // Manufacturer Information Record
    SequenceNo : String[1];
    InstrumentAlert : String[21];
    TestFlags : String[18];

    constructor Create;
    procedure SetASTMSegmentField(Idx:Byte; Val : String);override;
    function GetASTMSegmentField(Idx:Byte): String;override;
  end;

  TASTMCSegment = class(TASTMSegment)
    SequenceNo : String[1];
    CommentSource : String[1];
    CommentText : String[100];
    CommentType : String[1];
    constructor Create;
    procedure SetASTMSegmentField(Idx:Byte; Val : String);override;
    function GetASTMSegmentField(Idx:Byte): String;override;
  end;
 {}

var    ASTM_prevSeqNo : Byte;


implementation

uses StringUtils;
const
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



// ** ASTM ** //
function TASTMSegment.GetASTMSegment:String;
var Idx: Integer;
    str : string;
begin
  Result := '';
  Inc(ASTM_prevSeqNo);
  if ASTM_prevSeqNo > 7 then ASTM_prevSeqNo := 0;{}
  str := '';
  for Idx := 0 to FieldsNo do
    begin
      if str <> '' then str := str + FieldSeparator;
      str := str+GetASTMSegmentField(Idx);
    end;
  //str := str + FieldSeparator;
  str := IntToStr(ASTM_prevSeqNo) + str;
  str := str + CR + ETX;
  Result := STX + str+Format('%.2x',[CheckControl(str)])+CR+LF;
end;

procedure TASTMSegment.ParseASTMSegmentFromString(var ASTMString: String);
var Idx, Sep_pos : Byte;
    tmp_len : integer;
    tmp_str : string;
begin
  Idx := 0;
  Sep_pos := Pos(FieldSeparator, ASTMString);
  while ((Sep_pos > 0) and (Idx <= FieldsNo)) do
    begin
      tmp_str := Copy(ASTMString, 1, Sep_pos - 1);
      tmp_str := StrReplace(chr(10), '', tmp_str);
      tmp_str := StrReplace(chr(13), '', tmp_str);
      tmp_len := Length(ASTMString);
      ASTMString := Copy(ASTMString, Sep_pos + 1, tmp_len);
      SetASTMSegmentField(Idx, tmp_str);

      Inc(Idx);
      Sep_pos := Pos(FieldSeparator, ASTMString);
    end;
    If (Idx <= FieldsNo) then        // pune ultimul camp 
      SetASTMSegmentField(Idx, ASTMString);
end;

function TASTMSegment.CheckControl(Data: String): Integer;
var
  Val: Word;
  Idx: Integer;
begin
  Val := 0;
  for Idx := 1 to Length(Data) do
    Inc(Val, Ord(Data[Idx]));
  Result := (Val and $FF) mod 256;
end;

constructor TASTMHeaderSegment.Create;
begin
FieldsNo := 13;
FieldSeparator := '|';
SegmentName := 'H';
inherited;
end;

procedure TASTMHeaderSegment.SetASTMSegmentField(Idx:Byte; Val : String);
begin
  case Idx of
    0: SegmentName := Val;
    1: Delimiter := Val;
    2: MessageControlID := Val;
    3: AccessPassword := Val;
    4: SenderName := Val;
    5: SenderStrAddr := Val;
    6: ReservedField := Val;
    7: SenderPhoneNo := Val;
    8: SenderCharacteristics := Val;
    9: ReceiverID := Val;
    10: CommentSI := Val;
    11: Processing := Val;
    12: ASTMVer := Val;
    13: DateAndTime := Val;
  end;
end;

function TASTMHeaderSegment.GetASTMSegmentField(Idx:Byte): String;
begin
  case Idx of
    0: Result := SegmentName;
    1: Result := Delimiter;
    2: Result := MessageControlID;
    3: Result := AccessPassword;
    4: Result := SenderName;
    5: Result := SenderStrAddr;
    6: Result := ReservedField;
    7: Result := SenderPhoneNo;
    8: Result := SenderCharacteristics;
    9: Result := ReceiverID;
    10: Result := CommentSI;
    11: Result := Processing;
    12: Result := ASTMVer;
    13: Result := DateAndTime;
  end;
end;

constructor TASTMPIDSegment.Create;
begin
FieldsNo := 34;
FieldSeparator := '|';
SegmentName := 'P';
inherited;
end;

procedure TASTMPIDSegment.SetASTMSegmentField(Idx:Byte; Val : String);
begin
  case Idx of
    0: SegmentName := Val;
    1: SequenceNo := Val;
    2: PracticePatID := Val;
    3: LabPatID := Val;
    4: PatID3 := Val;
    5: Name := Val;
    6: MothersMName := Val;
    7: BirthDate := Val;
    8: if Length(Val) >= 1 then Sex := Val[1]
       else Sex := '';
    9: Race := Val;
    10: Address := Val;
    11: ResF1 := Val;
    12: Phone := Val;
    13: AttPhisician := Val;
    14: SpecField1 := Val;
    15: SpecField2 := Val;
    16: PacHeight := Val;
    17: PacWeight := Val;
    18: KnownDiag := Val;
    19: ActiveMed := Val;
    20: Diet := Val;
    21: PracticeF1 := Val;
    22: PracticeF2 := Val;
    23: AdmissionDate := Val;
    24: AdmissionStat := Val;
    25: Location := Val;
    26: NatureOfDiag := Val;
    27: AltDiagCode := Val;
    28: Religion := Val;
    29: MaritalStatus := Val;
    30: IsolationStatus := Val;
    31: Language := Val;
    32: HospService := Val;
    33: HospInst := Val;
    34: DosageCat := Val;
  end;
end;

function TASTMPIDSegment.GetASTMSegmentField(Idx:Byte): String;
begin
  case Idx of
    0: Result := SegmentName;
    1: Result := SequenceNo;
    2: Result := PracticePatID;
    3: Result := LabPatID;
    4: Result := PatID3;
    5: Result := Name;
    6: Result := MothersMName;
    7: Result := BirthDate;
    8: Result := Sex;
    9: Result := Race;
    10: Result := Address;
    11: Result := ResF1;
    12: Result := Phone;
    13: Result := AttPhisician;
    14: Result := SpecField1;
    15: Result := SpecField2;
    16: Result := PacHeight;
    17: Result := PacWeight;
    18: Result := KnownDiag;
    19: Result := ActiveMed;
    20: Result := Diet;
    21: Result := PracticeF1;
    22: Result := PracticeF2;
    23: Result := AdmissionDate;
    24: Result := AdmissionStat;
    25: Result := Location;
    26: Result := NatureOfDiag;
    27: Result := AltDiagCode;
    28: Result := Religion;
    29: Result := MaritalStatus;
    30: Result := IsolationStatus;
    31: Result := Language;
    32: Result := HospService;
    33: Result := HospInst;
    34: Result := DosageCat;
  end;
end;

constructor TASTMResRecSegment.Create;
begin
FieldsNo := 13;
FieldSeparator := '|';
SegmentName := 'R';
inherited;
end;

procedure TASTMResRecSegment.SetASTMSegmentField(Idx:Byte; Val : String);
begin
  case Idx of
    0: SegmentName := Val;
    1: SequenceNo := Val;
    2: UniversalTestID := Val;
    3: DataValue := Val;
    4: UnitsOfMeas := Val;
    5: RefRang := Val;
    6: Flags := Val;
    7: NatureOfAbn := Val;
    8: ResStatus := Val;
    9: DateOfChange := Val;
    10: OperatorID := Val;
    11: DateTimeTestStarted := Val;
    12: DateTimeTestCompl := Val;
    13: InstrumentID := Val;
  end;
end;

function TASTMResRecSegment.GetASTMSegmentField(Idx:Byte): String;
begin
  case Idx of
    0: Result := SegmentName;
    1: Result := SequenceNo;
    2: Result := UniversalTestID;
    3: Result := DataValue;
    4: Result := UnitsOfMeas;
    5: Result := RefRang;
    6: Result := Flags;
    7: Result := NatureOfAbn;
    8: Result := ResStatus;
    9: Result := DateOfChange;
    10: Result := OperatorID;
    11: Result := DateTimeTestStarted;
    12: Result := DateTimeTestCompl;
    13: Result := InstrumentID;
  end;
end;


constructor TASTMMTRSegment.Create;
begin
FieldsNo := 2;
FieldSeparator := '|';
SegmentName := 'L';
inherited;
end;

procedure TASTMMTRSegment.SetASTMSegmentField(Idx:Byte; Val : String);
begin
  case Idx of
    0: SegmentName := Val;
    1: SequenceNo := Val;
    2: TermCode := Val;
  end;
end;

function TASTMMTRSegment.GetASTMSegmentField(Idx:Byte): String;
begin
  case Idx of
    0: Result := SegmentName;
    1: Result := SequenceNo;
    2: Result := TermCode;
  end;
end;

constructor TASTMRIRSegment.Create;
begin
FieldsNo := 12;
FieldSeparator := '|';
SegmentName := 'Q';
inherited;
end;

procedure TASTMRIRSegment.SetASTMSegmentField(Idx:Byte; Val : String);
begin
  case Idx of
    0: SegmentName := Val;
    1: SequenceNo := Val;
    2: StartingRangeID := Val;
    3: EndingRangeID := Val;
    4: UniversalTestId := Val;
    5: NatureReqTL := Val;
    6: BegReqResDateTime := Val;
    7: EndReqResDateTime := Val;
    8: ReqPhysicianName := Val;
    9: ReqPhysicianPhone := Val;
    10: UserFld1 := Val;
    11: UserFld2 := Val;
    12: ReqInfoStatus := Val;
  end;
end;

function TASTMRIRSegment.GetASTMSegmentField(Idx:Byte): String;
begin
  case Idx of
    0: Result := SegmentName;
    1: Result := SequenceNo;
    2: Result := StartingRangeID;
    3: Result := EndingRangeID;
    4: Result := UniversalTestId;
    5: Result := NatureReqTL;
    6: Result := BegReqResDateTime;
    7: Result := EndReqResDateTime;
    8: Result := ReqPhysicianName;
    9: Result := ReqPhysicianPhone;
    10: Result := UserFld1;
    11: Result := UserFld2;
    12: Result := ReqInfoStatus;
  end;
end;

constructor TASTMTORSegment.Create;
begin
FieldsNo := 30;
FieldSeparator := '|';
SegmentName := 'O';
inherited;
end;

procedure TASTMTORSegment.SetASTMSegmentField(Idx:Byte; Val : String);
begin
  case Idx of
    0: SegmentName := Val;
    1: SequenceNo := Val;
    2: SampleID := Trim(Val);
    3: InstrSpecimenID := Val;
    4: UniversalTestID := Val;
    5: Priority := Val;
    6: RequestedDateTime := Val;
    7: SpecimentCollectDateTime := Val;
    8: CollectionEndTime := Val;
    9: CollectionVol := Val;
    10: CollectorID := Val;
    11: ActionCode := Val;
    12: DangerCode := Val;
    13: RelevantClInfo := Val;
    14: DateTimeSpecRecvd := Val;
    15: SpecimenType := Val;
    16: OrderingPhysician := Val;
    17: PhysiscianPhone := Val;
    18: UserFld1 := Val;
    19: UserFld2 := Val;
    20: LabFld1 := Val;
    21: LabFld2 := Val;
    22: DateTimeResRep := Val;
    23: InstrumentCharge := Val;
    24: InstrumentSectionID := Val;
    25: RecordType := Val;
    26: ResFld:= Val;
    27: LocOfSpecCol := Val;
    28: NosocomialInfFlg := Val;
    29: SpecimenService := Val;
    30: SpecimenInst := Val;
  end;
end;

function TASTMTORSegment.GetASTMSegmentField(Idx:Byte): String;
begin
  case Idx of
    0: Result := SegmentName;
    1: Result := SequenceNo;
    2: Result := SampleID;
    3: Result := InstrSpecimenID;
    4: Result := UniversalTestID;
    5: Result := Priority;
    6: Result := RequestedDateTime;
    7: Result := SpecimentCollectDateTime;
    8: Result := CollectionEndTime;
    9: Result := CollectionVol;
    10: Result := CollectorID;
    11: Result := ActionCode;
    12: Result := DangerCode;
    13: Result := RelevantClInfo;
    14: Result := DateTimeSpecRecvd;
    15: Result := SpecimenType;
    16: Result := OrderingPhysician;
    17: Result := PhysiscianPhone;
    18: Result := UserFld1;
    19: Result := UserFld2;
    20: Result := LabFld1;
    21: Result := LabFld2;
    22: Result := DateTimeResRep;
    23: Result := InstrumentCharge;
    24: Result := InstrumentSectionID;
    25: Result := RecordType;
    26: Result := ResFld;
    27: Result := LocOfSpecCol;
    28: Result := NosocomialInfFlg;
    29: Result := SpecimenService;
    30: Result := SpecimenInst;
  end;
end;

{  TASTMMIRSegment  }
constructor TASTMMIRSegment.Create;
begin
FieldsNo := 4;
FieldSeparator := '|';
SegmentName := 'M';
inherited;
end;

procedure TASTMMIRSegment.SetASTMSegmentField(Idx:Byte; Val : String);
begin
  case Idx of
    0: SegmentName := Val;
    1: SequenceNo := Val;
    2: InstrumentAlert := Val;
    3: TestFlags := Val;
  end;
end;

function TASTMMIRSegment.GetASTMSegmentField(Idx:Byte): String;
begin
  case Idx of
    0: Result := SegmentName;
    1: Result := SequenceNo;
    2: Result := InstrumentAlert;
    3: Result := TestFlags;
  end;
end;

{  TASTMCSegment  }
constructor TASTMCSegment.Create;
begin
FieldsNo := 5;
FieldSeparator := '|';
SegmentName := 'C';
inherited;
end;

procedure TASTMCSegment.SetASTMSegmentField(Idx:Byte; Val : String);
begin
  case Idx of
    0: SegmentName := Val;
    1: SequenceNo := Val;
    2: CommentSource := Val;
    3: CommentText := Val;
    4: CommentType := Val;
  end;
end;

function TASTMCSegment.GetASTMSegmentField(Idx:Byte): String;
begin
  case Idx of
    0: Result := SegmentName;
    1: Result := SequenceNo;
    2: Result := CommentSource;
    3: Result := CommentText;
    4: Result := CommentType;
  end;
end;

end.
