using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using System.IO;
// Test-only serializer shim: the production host is still compiled against net48.
namespace System.Web.Script.Serialization {public class JavaScriptSerializer {public int MaxJsonLength{get;set;}public int RecursionLimit{get;set;}public string Serialize(object o)=>System.Text.Json.JsonSerializer.Serialize(o);public T Deserialize<T>(string s)=>System.Text.Json.JsonSerializer.Deserialize<T>(s);public object Deserialize(string s,Type t)=>System.Text.Json.JsonSerializer.Deserialize(s,t);}}
public enum CoduriCampuriCard{A1,C1}
public enum CoduriRaspunsOperatieCamp{OK=0,Denied=-1}
public enum CoduriRaspunsOperatieCard{OK=0,ERR_AUTENTIFICARE=-11,ERR_TOKEN_LIPSA=-1,ERR_TOKEN_INVALID=-2,ERR_TOKEN_RESETAT=-12,ERR_UM_TOKEN=-27,ERR_UM_INDISPONIBILA=-29,ERR_UM_PROCESARE=-44,ERR_INVALID_PIN=-14,ERR_UM_STARE_CARD_INVALIDA=-5}
public class MesajeRaspunsCard:Hashtable{public MesajeRaspunsCard(){this[CoduriRaspunsOperatieCard.ERR_UM_INDISPONIBILA]="unavailable";this[CoduriRaspunsOperatieCard.ERR_INVALID_PIN]="wrong pin";}}
public class IdentificatorDrepturi{public string CUI{get;set;}public string NumarContract{get;set;}public DateTime DataContract{get;set;}public string CasaAsigurare{get;set;}public string TipFurnizor{get;set;}}
public class Camp{public CoduriCampuriCard Cod{get;set;}public object Valoare{get;set;}}
public class AdminData{public Camp NumarAsigurat{get;set;}=new Camp{Cod=CoduriCampuriCard.C1,Valoare=""};public Camp Name{get;set;}=new Camp{Cod=CoduriCampuriCard.A1,Valoare="test"};}
public class CardData{public AdminData DateAdministrative{get;set;}=new AdminData();}
public class Profile{public int ProfilCard{get;set;}=2;}
public class TerminalData{public string TerminalId{get;set;}="test-terminal";public List<Profile> Profile{get;set;}=new List<Profile>{new Profile()};}
public class FakeManager{public int TemplatePreparations;public void EnsureCardTemplate(){TemplatePreparations++;}public string EditarePeTerminal{get;set;}="AUTENTIFICARE";public int? ProfilId{get;set;}public TerminalData TerminalData{get;set;}=new TerminalData();public IList RaspunsuriUMPentruOffline{get;set;}=new List<CoduriRaspunsOperatieCard>();public int CIDReads;
 private int CitesteDate(List<CoduriCampuriCard> fields,CardData data,Dictionary<CoduriCampuriCard,CoduriRaspunsOperatieCamp> results){CIDReads++;data.DateAdministrative.NumarAsigurat.Valoare="cid-test";results[CoduriCampuriCard.C1]=CoduriRaspunsOperatieCamp.OK;return 0;}}
public class FakeSession{public FakeManager TerminalManager{get;set;}=new FakeManager();public string Token{get;set;}="token";public string StareAutentificare{get;set;}="AUTENTIFICAT";public bool TerminalConectat{get;set;}=true;public string StareCardInTerminal{get;set;}="CardInserat";public bool ChangeCard;public int Reads,Tokens;public bool Partial,Offline,WrongToken,AlwaysBad;public int FirstCode;
 public string ObtineToken(string cif,IdentificatorDrepturi rights){Tokens++;if(Offline)throw new Exception("unavailable");if(WrongToken)throw new Exception("wrong pin");return Token="new-token";}
 public int CitesteDate(string token,List<CoduriCampuriCard> fields,ref CardData data,ref Dictionary<CoduriCampuriCard,CoduriRaspunsOperatieCamp> results){Reads++;if(ChangeCard)typeof(ECardHost).GetField("Generation",BindingFlags.Static|BindingFlags.NonPublic).SetValue(null,1);foreach(var f in fields)results[f]=Partial?CoduriRaspunsOperatieCamp.Denied:CoduriRaspunsOperatieCamp.OK;return Reads==1||AlwaysBad?FirstCode:0;}
 public int CitesteCertificate(){return 0;}}
public enum TestCardState {CardInserat,CardRetras}
public class InitializingSession:FakeSession {
 public event Action<TestCardState,CoduriRaspunsOperatieCard> DupaStareCardInTerminalSchimbata;
 readonly object stateLock=new object();
 bool initialized;
 public int Initializations;
 public CoduriRaspunsOperatieCard InitializationResult;
 public readonly System.Threading.ManualResetEventSlim Entered=new System.Threading.ManualResetEventSlim();
 public readonly System.Threading.ManualResetEventSlim Release=new System.Threading.ManualResetEventSlim();
 public void BeginInsertion(){lock(stateLock){
  Entered.Set();Release.Wait();Initialize();
 }}
 void Initialize(){if(initialized)return;initialized=true;Initializations++;
  // Reproduce the SDK's misleading offline state even on a negative result.
  StareAutentificare="AUTENTIFICAT_OFFILINE";
  DupaStareCardInTerminalSchimbata?.Invoke(TestCardState.CardInserat,InitializationResult);
 }
 public void SchimbareStareCardInTerminal(TestCardState state){lock(stateLock){Initialize();}}
}
public class LateNotifyingSession:InitializingSession {
 public int Notifications;
 public int? ProfileWhenAttached;
 public event Action<TestCardState> StareCardInTerminalSchimbata {
  add {Notifications++;ProfileWhenAttached=TerminalManager.ProfilId;value(TestCardState.CardInserat);}
  remove {}
 }
}
public class HostTests{
 static Type H=typeof(ECardHost);static void Set(string n,object v)=>H.GetField(n,BindingFlags.Static|BindingFlags.NonPublic).SetValue(null,v);static object Get(string n)=>H.GetField(n,BindingFlags.Static|BindingFlags.NonPublic).GetValue(null);
 static object Call(string n,params object[] args){try{return H.GetMethod(n,BindingFlags.Static|BindingFlags.NonPublic).Invoke(null,args);}catch(TargetInvocationException e){throw e.InnerException;}}
 static Dictionary<string,object> Contract()=>new Dictionary<string,object>{{"cif","1"},{"cui","1"},{"number","test"},{"date","2023-06-30"},{"house","CAS"},{"provider_type","PAR"}};
 static FakeSession Setup(){var s=new FakeSession();Set("Wrapper",null);Set("WrapperMode",false);Set("SDK",typeof(HostTests).Assembly);Set("Session",s);Set("Output",TextWriter.Null);Set("Current",new Dictionary<string,object>{{"operation","read"},{"contract",Contract()}});Set("Generation",0);Set("Token","token");Set("OfflineToken",false);Set("TokenPrepared",false);Set("CardNotificationsAttached",false);return s;}
 static void Check(bool ok,string name){if(!ok)throw new Exception(name);Console.WriteLine("PASS "+name);}
 static bool Fails(Action f){try{f();return false;}catch{return true;}}
 public static int Main(string[] args){
 Setup();var lateSession=new LateNotifyingSession{Offline=true};Set("Session",lateSession);Call("HookCardInitialization");
 Check(Fails(()=>Call("AttachPreparedCardNotifications"))&&lateSession.Notifications==0,"no insertion auto-authentication before token preparation");
 Call("AcquireToken",Contract());Check(lateSession.TerminalManager.TemplatePreparations==1,"card template detected before acquiring token without insertion subscription");Call("AttachPreparedCardNotifications");Call("AttachPreparedCardNotifications");Call("CompleteCardInitialization");
 Check(lateSession.Notifications==1&&lateSession.ProfileWhenAttached==2,"insertion subscription occurs once after offline profile is ready");
 Check(lateSession.Initializations==1,"prepared card initialized once before read");
 Setup();lateSession=new LateNotifyingSession{WrongToken=true};Set("Session",lateSession);
 Check(Fails(()=>Call("AcquireToken",Contract()))&&Fails(()=>Call("AttachPreparedCardNotifications"))&&lateSession.Notifications==0,"failed token preparation never starts insertion authentication");

 foreach(var code in new[]{CoduriRaspunsOperatieCard.OK,CoduriRaspunsOperatieCard.ERR_AUTENTIFICARE,CoduriRaspunsOperatieCard.ERR_INVALID_PIN}){
  Setup();var initializing=new InitializingSession{InitializationResult=code};Set("Session",initializing);Call("HookCardInitialization");
  var insertion=System.Threading.Tasks.Task.Run(()=>initializing.BeginInsertion());
  Check(initializing.Entered.Wait(2000),"SDK insertion callback started");
  var barrierEntered=new System.Threading.ManualResetEventSlim();
  var barrier=System.Threading.Tasks.Task.Run(()=>{barrierEntered.Set();return Fails(()=>Call("CompleteCardInitialization"));});
  barrierEntered.Wait();Check(!barrier.Wait(50),"read waits for active SDK insertion authentication");
  initializing.Release.Set();Check(insertion.Wait(2000)&&barrier.Wait(2000),"initialization barrier completes");
  Check(barrier.Result==(code!=CoduriRaspunsOperatieCard.OK),"offline state never hides initialization result "+code);
  Check(initializing.Initializations==1,"initialization authentication is not repeated");
 }

 var inputSession=Setup();
 foreach(var mode in new[]{"AUTENTIFICARE","SCHIMBARE_PIN","RESETARE_PIN","SCHIMBARE_PIN_TRANSPORT"}){
  inputSession.TerminalManager.EditarePeTerminal=mode;
  Check((string)Call("AuthenticationInputKind")==mode,"preserve SDK terminal input mode "+mode);
 }

 var s=Setup();s.FirstCode=-2;Call("Read",new string[]{"A1"},true);Check(s.Reads==2&&s.Tokens==1,"native SDK refreshes token once using CNAS compatibility");
 s=Setup();s.FirstCode=-2;s.AlwaysBad=true;Check(Fails(()=>Call("Read",new string[]{"A1"},true))&&s.Reads==2,"native SDK token recovery does not loop");
 s=Setup();s.FirstCode=-14;Check(Fails(()=>Call("Read",new string[]{"A1"},true))&&s.Tokens==0&&s.Reads==1,"wrong PIN not replayed inside SDK");
 s=Setup();s.Partial=true;Call("Read",new string[]{"A1"},false);Check(s.Reads==1,"partial read retained");Check(Fails(()=>Call("Read",new string[]{"A1"},true)),"sign identity remains strict");
 s=Setup();var result=Call("Read",new string[]{"C1"},true) as CardData;Check(result.DateAdministrative.NumarAsigurat.Valoare.ToString()=="cid-test"&&s.TerminalManager.CIDReads==1,"native SDK recovers CID after authentication");
 s=Setup();s.StareAutentificare="AUTENTIFICAT_OFFILINE";result=Call("Read",new string[]{"C1"},true) as CardData;Check(result.DateAdministrative.NumarAsigurat.Valoare.ToString()=="cid-test"&&s.TerminalManager.CIDReads==1,"successful offline read accepts actual SDK authentication enum for CID recovery");
 s=Setup();s.StareAutentificare="AUTENTIFICAT_OFFILINE";s.FirstCode=-11;Check(Fails(()=>Call("Read",new string[]{"C1"},true))&&s.TerminalManager.CIDReads==0&&s.Reads==1,"misleading offline state never triggers CID recovery after authentication failure");
 s=Setup();s.Offline=true;Call("AcquireToken",Contract());Check(s.TerminalManager.ProfilId==2&&s.Token==new string('0',32)&&(bool)Get("OfflineToken"),"native SDK falls back offline on UM unavailable without firewall");
 s=Setup();s.Offline=true;s.TerminalManager.TerminalData.Profile.Clear();Check(Fails(()=>Call("AcquireToken",Contract())),"offline still requires an enrolled profile");
 s=Setup();Call("ConfigureCNASOfflineResponses");Call("ConfigureCNASOfflineResponses");Check(s.TerminalManager.RaspunsuriUMPentruOffline.Count==2,"native SDK offline response configuration is idempotent");
 s=Setup();s.WrongToken=true;Check(Fails(()=>Call("AcquireToken",Contract())),"wrong PIN never becomes offline success");
 s=Setup();s.Offline=true;Set("Current",new Dictionary<string,object>{{"operation","enroll"}});Check(Fails(()=>Call("AcquireToken",Contract())),"enrollment never becomes offline success");

 Check((string)Call("ChooseReader","Reader 1",new string[]{"Reader"},new string[]{"Reader 0","Reader 1"})=="Reader 1","physical reader preserved");
 Check(Fails(()=>Call("ChooseReader","Reader",new string[]{"Reader"},new string[]{"Reader 0","Reader 1"})),"ambiguous model rejected");
 Check((string)Call("ChooseReader","",new string[]{"Reader"},new string[]{"Reader 0"})=="Reader 0","single reader auto selection");
 s=Setup();s.Partial=true;Check(Fails(()=>Call("ReadResponse",(object)new string[]{"A1"})),"all requested fields failed is not success");
 s=Setup();s.ChangeCard=true;Check(Fails(()=>Call("Read",new string[]{"A1"},false)),"changed card invalidates read result");
 s=Setup();Call("WaitForReader",true);Check(s.Reads==0,"ready card wait does not perform operations");
 var wrapperSession=Setup();var adapter=new CnasWrapperAdapter(typeof(HostTests).Assembly.Location,wrapperSession,Contract(),(name,data)=>{});
 Check(adapter.AcquireToken()=="fixture-token","adapter invokes loaded wrapper token method");
 wrapperSession.Offline=true;Check(adapter.AcquireToken()==new string('0',32),"offline token comes from loaded wrapper");
 int calls=0;Check(adapter.Evaluate(()=>++calls==1?-2:0)==0&&calls==2,"adapter delegates token recovery to wrapper evaluator");
 Check(Cnas.Ceas.SmartCard.Novensys.Services.NovensysSmartCardContext.Refreshes==1,"wrapper refresh method ran");
 adapter.Execute("activate");Check(Cnas.Ceas.SmartCard.Novensys.Services.NovensysSmartCardContext.Activations==1,"adapter delegates explicit activation to wrapper");
 Check(adapter.Sign(new byte[]{1,2}).Length==2&&Cnas.Ceas.SmartCard.Novensys.Services.NovensysSmartCardContext.Signs==1,"adapter delegates signing to wrapper");
 Check(Cnas.Ceas.SmartCard.Novensys.Services.TerminalManagerInterceptor.Calls==1,"adapter invokes original wrapper interceptor");
 Cnas.Ceas.SmartCard.Novensys.Services.NovensysSmartCardContext.FailConstruction=true;
 try{
  Set("WrapperMode",true);Set("Stage","cnas_wrapper_initialize");
  Exception bootError=null;var steps=new List<string>();
  try{new CnasWrapperAdapter(typeof(HostTests).Assembly.Location,wrapperSession,Contract(),(name,data)=>{steps.Add(name);});}catch(Exception ex){bootError=ex;}
  Check(bootError!=null&&CnasWrapperAdapter.InitializationStep=="construct_cnas_context","failed CNAS constructor identifies its initialization step");
  var diagnostics=Call("ExceptionDiagnostics",bootError);var diagnosticType=diagnostics.GetType();
  Check(Convert.ToInt32(diagnosticType.GetProperty("native_error_code").GetValue(diagnostics))==5,"preserve Win32 native error code");
  Check((string)diagnosticType.GetProperty("native_message").GetValue(diagnostics)=="fixture Windows initialization error","preserve Win32 message");
  Check(steps.Contains("cnas_wrapper_initializing"),"publish constructor steps before failure");
  Cnas.Ceas.SmartCard.Novensys.Services.NovensysSmartCardContext.FailConstruction=false;
  Cnas.Ceas.SmartCard.Novensys.Services.NovensysSmartCardContext.FailWithUnthrownInner=true;
  bootError=null;
  try{new CnasWrapperAdapter(typeof(HostTests).Assembly.Location,wrapperSession,Contract(),(name,data)=>{});}catch(Exception ex){bootError=ex;}
  Check(bootError!=null&&String.IsNullOrEmpty(bootError.GetBaseException().StackTrace),"reproduce error 233 with empty innermost stack");
  diagnostics=Call("ExceptionDiagnostics",bootError);diagnosticType=diagnostics.GetType();
  Check(Convert.ToInt32(diagnosticType.GetProperty("native_error_code").GetValue(diagnostics))==233,"preserve native pipe error 233");
  Check(((string[])diagnosticType.GetProperty("stack_frames").GetValue(diagnostics)).Length>0,"retain outer stack when root stack is empty");
  var exceptionChain=(List<object>)diagnosticType.GetProperty("exception_chain").GetValue(diagnostics);bool foundType=false;
  foreach(var item in exceptionChain){if(Convert.ToString(item.GetType().GetProperty("type_name").GetValue(item))=="Fixture.CnasShell")foundType=true;}
  Check(foundType,"retain failing static initialization type");
 }finally{Cnas.Ceas.SmartCard.Novensys.Services.NovensysSmartCardContext.FailWithUnthrownInner=false;Cnas.Ceas.SmartCard.Novensys.Services.NovensysSmartCardContext.FailConstruction=false;Set("WrapperMode",false);}
 Cnas.Ceas.SmartCard.Novensys.Services.NovensysSmartCardContext.RequireRegisteredCredentials=true;
 try {
  wrapperSession=Setup();CnasCredentialsFixture.Container=new Microsoft.Practices.Unity.UnityContainer();CnasCredentialsFixture.SQLFallbacks=0;
  Check(Fails(()=>new CnasWrapperAdapter(typeof(HostTests).Assembly.Location,wrapperSession,Contract(),(name,data)=>{})),"unregistered credentials reproduce desktop SQL dependency");
  var register=typeof(CnasWrapperAdapter).GetMethod("RegisterStandaloneCredentials",BindingFlags.NonPublic|BindingFlags.Static);
  var instance=register.Invoke(null,new object[]{CnasCredentialsFixture.Container,typeof(Cnas.Siui.WebServices.Interfaces.ICredentialsProvider)});
  Check(instance!=null&&Object.ReferenceEquals(instance,CnasCredentialsFixture.Container.Resolve(typeof(Cnas.Siui.WebServices.Interfaces.ICredentialsProvider))),"register standalone credentials with original Unity contract");
  adapter=new CnasWrapperAdapter(typeof(HostTests).Assembly.Location,wrapperSession,Contract(),(name,data)=>{});
  Check(CnasCredentialsFixture.SQLFallbacks==1&&adapter.AcquireToken()=="fixture-token","registered credentials prevent constructor SQL fallback and preserve wrapper token call");
  Check(Fails(()=>((Cnas.Siui.WebServices.Interfaces.ICredentialsProvider)instance).GetServiceUri(null)),"unconfigured SIUI HTTPS is rejected explicitly");
  Check(Fails(()=>((Cnas.Siui.WebServices.Interfaces.ICredentialsProvider)instance).GetServerToken(null)),"no fake SIUI server token is supplied");
 } finally {Cnas.Ceas.SmartCard.Novensys.Services.NovensysSmartCardContext.RequireRegisteredCredentials=false;}
 if(args.Length==2&&args[0]=="--vendor-credentials"){
  var directory=Path.GetFullPath(args[1]);
  AppDomain.CurrentDomain.AssemblyResolve+=(sender,e)=>{var path=Path.Combine(directory,new AssemblyName(e.Name).Name+".dll");return File.Exists(path)?Assembly.LoadFrom(path):null;};
  var unityAssembly=Assembly.LoadFrom(Path.Combine(directory,"Microsoft.Practices.Unity.dll"));
  var vendorContainer=Activator.CreateInstance(unityAssembly.GetType("Microsoft.Practices.Unity.UnityContainer",true));
  var web=Assembly.LoadFrom(Path.Combine(directory,"Cnas.Siui.WebServices.dll"));
  var credentialType=web.GetType("Cnas.Siui.WebServices.Interfaces.ICredentialsProvider",true);
  var register=typeof(CnasWrapperAdapter).GetMethod("RegisterStandaloneCredentials",BindingFlags.NonPublic|BindingFlags.Static);
  var instance=register.Invoke(null,new object[]{vendorContainer,credentialType});
  Check(credentialType.IsInstanceOfType(instance),"original vendor credentials interface implemented without SQL");
  var unity=unityAssembly.GetType("Microsoft.Practices.Unity.IUnityContainer",true);
  var resolverOverride=unityAssembly.GetType("Microsoft.Practices.Unity.ResolverOverride",true);
  var resolved=unity.GetMethod("Resolve").Invoke(vendorContainer,new object[]{credentialType,null,Array.CreateInstance(resolverOverride,0)});
  Check(Object.ReferenceEquals(instance,resolved),"original vendor Unity resolves injected credentials instance");
  bool defaultRegistration=false;
  foreach(var registration in (System.Collections.IEnumerable)unity.GetProperty("Registrations").GetValue(vendorContainer,null)){
   var t=registration.GetType();if((Type)t.GetProperty("RegisteredType").GetValue(registration,null)==credentialType&&t.GetProperty("Name").GetValue(registration,null)==null)defaultRegistration=true;
  }
  Check(defaultRegistration,"default vendor registration matches CNAS SafeResolve lookup");
 }
 return 0;}}
