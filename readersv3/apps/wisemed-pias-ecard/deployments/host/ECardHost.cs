// Isolated .NET Framework host. One SDK assembly and one session per process.
// Protocol: JSON lines over inherited anonymous stdin/stdout pipes, never TCP.
using System;
using System.IO;
using System.Text;
using System.Linq;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using System.Runtime.InteropServices;
using System.Linq.Expressions;
using System.Security.Cryptography;
using System.Security.Cryptography.X509Certificates;
using System.Security.Cryptography.Pkcs;
using System.Threading;
using System.ComponentModel;
using System.Diagnostics;
using System.Web.Script.Serialization;

public static class ECardHost {
 static string HostVersion {get {var a=(AssemblyInformationalVersionAttribute)Attribute.GetCustomAttribute(typeof(ECardHost).Assembly,typeof(AssemblyInformationalVersionAttribute));return a==null?"dev":a.InformationalVersion;}}
 static readonly JavaScriptSerializer Json = new JavaScriptSerializer { MaxJsonLength = 2 * 1024 * 1024, RecursionLimit = 64 };
 static TextWriter Output;
 static Assembly SDK;
 static bool WrapperMode;
 static string SDKFingerprint;
 static CnasWrapperAdapter Wrapper;
 static object Session;
 static string Token;
 static string Stage;
 static IDictionary LastFieldResults;
 static bool OfflineToken;
 static bool TokenPrepared;
 static bool CardNotificationsAttached;
 static int Generation;
 static int PinnedGeneration;
 static bool PinAttempted;
 static int CardInitializationCode;
 static string CertificateHash;
 static bool Opened;
 static volatile bool ActiveCommand;
 static X509Certificate2 PinnedCertificate;
 static readonly object InputLock = new object();
 static Dictionary<string,object> Current;
 static readonly List<Delegate> Hooks = new List<Delegate>();
 static Type TypeNamed(string name) { return SDK.GetTypes().Single(t => t.Name == name); }
 static object Get(object obj,string name) { if(obj==null)return null; var p=obj.GetType().GetProperty(name,BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic); return p==null?null:p.GetValue(obj,null); }
 static void Set(object obj,string name,object value) { obj.GetType().GetProperty(name).SetValue(obj,value,null); }
 static string Str(object obj) {return obj==null?"":Convert.ToString(obj,System.Globalization.CultureInfo.InvariantCulture);}
 static string Text(Dictionary<string,object> d,string key) {object v;return d.TryGetValue(key,out v)?Str(v):"";}
 static Dictionary<string,object> Dict(Dictionary<string,object>d,string key){object v;return d.TryGetValue(key,out v)?(Dictionary<string,object>)v:new Dictionary<string,object>();}
 static object Invoke(object obj,string name,params object[] args){Stage=name;var t=obj as Type ?? obj.GetType();var methods=t.GetMethods().Where(m=>m.Name==name&&m.GetParameters().Length==args.Length).ToArray();if(methods.Length!=1)throw new HostError("sdk_incompatible",0);return methods[0].Invoke(obj is Type?null:obj,args);}
 static void Check(object code){int n=Convert.ToInt32(code);if(n!=0)throw new HostError("sdk_error",n);}
 static void Send(object result){lock(Output){Output.WriteLine(Json.Serialize(result));Output.Flush();}}
 static object SDKIdentity(){return new {adapter=WrapperMode?"cnas_wrapper":"official_sdk",assembly_version=SDK==null?null:SDK.GetName().Version.ToString(),dll_sha256=SDKFingerprint,wrapper_version=Wrapper==null?null:Wrapper.Version,wrapper_sha256=Wrapper==null?null:Wrapper.SHA256};}
 static object Result(object data){return new {ok=true,data=data,pin_attempted=PinAttempted,host_version=HostVersion,sdk_identity=SDKIdentity()};}
 static void Hook(string name,Action<object[]> handler){
  var ev=Session.GetType().GetEvent(name);if(ev==null)throw new HostError("sdk_incompatible",0);
  var pars=ev.EventHandlerType.GetMethod("Invoke").GetParameters().Select(p=>Expression.Parameter(p.ParameterType,p.Name)).ToArray();
  var array=Expression.NewArrayInit(typeof(object),pars.Select(p=>Expression.Convert(p,typeof(object))));
  var body=Expression.Call(Expression.Constant(handler),typeof(Action<object[]>).GetMethod("Invoke"),array);
  var del=Expression.Lambda(ev.EventHandlerType,body,pars).Compile();ev.AddEventHandler(Session,del);Hooks.Add(del);
 }
 static void Diagnostic(string name,object data){if(ActiveCommand)Send(new {@event=name,data=data});}
 static void HookDiagnostic(string name){
  if(Session.GetType().GetEvent(name)==null){Diagnostic("event_unavailable",new {name=name});return;}
  Hook(name,args=>{
   var values=new List<string>();
   foreach(var arg in args){if(arg==null)continue;var t=arg.GetType();if(t.IsEnum||arg is bool||arg is int)values.Add(Str(arg));}
   Diagnostic(name,new {states=values});
  });
 }
 static string AuthenticationInputKind(){
  try {return Str(Get(Get(Session,"TerminalManager"),"EditarePeTerminal"));}
  catch {return "UNKNOWN";}
 }
 static void UserInput(object[] args){
  lock(InputLock){var e=args[args.Length-1];if(!ActiveCommand){Set(e,"Cancel",true);return;}PinAttempted=true;Send(new { @event="pin", prompt=Str(Get(e,"Prompt")), input_kind=AuthenticationInputKind() });
   var line=Console.ReadLine();if(line==null){Set(e,"Cancel",true);return;}
   var reply=Json.Deserialize<Dictionary<string,object>>(line);var value=Text(reply,"value");if(value.Length==0){Set(e,"Cancel",true);return;}Set(e,"Result",value);value=null;
  }
 }
 static void DisableSDKLogs(){
  // These vendor logs can contain APDUs/PINs. An invalid directory disables
  // their FileLog sink (vendor catches its IO exceptions). No patient logs.
  var t=SDK.GetTypes().FirstOrDefault(x=>x.Name=="LogManager");if(t==null)return;
  var p=t.GetProperty("LogFolderPath",BindingFlags.Static|BindingFlags.NonPublic|BindingFlags.Public);
  if(p!=null&&p.CanWrite)p.SetValue(null,"\0",null);
 }
 static bool IsCode(object code,params string[] names){var name=SDKCodeName(Convert.ToInt32(code));return names.Contains(name);}
 static object InvokePrivate(object obj,string name,params object[] args){
  for(var t=obj.GetType();t!=null;t=t.BaseType){var methods=t.GetMethods(BindingFlags.Instance|BindingFlags.NonPublic|BindingFlags.Public|BindingFlags.DeclaredOnly).Where(m=>m.Name==name&&m.GetParameters().Length==args.Length).ToArray();if(methods.Length==1){Stage=name;return methods[0].Invoke(obj,args);}}
  throw new HostError("sdk_incompatible",0);
 }
 static void ConfigureCNASOfflineResponses(){
  var manager=Get(Session,"TerminalManager");var list=Get(manager,"RaspunsuriUMPentruOffline") as IList;
  if(list==null){Diagnostic("cnas_offline_compatibility_unavailable",null);return;}
  var type=TypeNamed("CoduriRaspunsOperatieCard");
  foreach(var name in new[]{"ERR_UM_STARE_CARD_INVALIDA","ERR_UM_PROCESARE"})if(Enum.IsDefined(type,name)){var value=Enum.Parse(type,name);if(!list.Contains(value))list.Add(value);}
  Diagnostic("cnas_offline_compatibility_enabled",null);
 }
 static object Read(string[] fields,bool strict=true){
  int generation=Volatile.Read(ref Generation);
  var et=TypeNamed("CoduriCampuriCard");var list=(IList)Activator.CreateInstance(typeof(List<>).MakeGenericType(et));
  foreach(var f in fields){if(!Enum.IsDefined(et,f))throw new HostError("sdk_incompatible",0);list.Add(Enum.Parse(et,f));}
  var data=Activator.CreateInstance(TypeNamed("CardData"));
  var results=Activator.CreateInstance(typeof(Dictionary<,>).MakeGenericType(et,TypeNamed("CoduriRaspunsOperatieCamp")));
  object[] args={Token,list,data,results};
  Func<int> read=()=>{if(generation!=Volatile.Read(ref Generation))throw new HostError("card_changed",-61);if(Wrapper!=null)args[0]=Wrapper.Token;return Convert.ToInt32(Invoke(Session,"CitesteDate",args));};
  var readCode=Wrapper==null?read():Wrapper.Evaluate(read);
  if(Wrapper==null&&IsCode(readCode,"ERR_TOKEN_LIPSA","ERR_TOKEN_INVALID","ERR_TOKEN_RESETAT","ERR_UM_TOKEN")){
   Diagnostic("cnas_token_refresh",new {sdk_code=readCode});AcquireToken(Dict(Current,"contract"));
   if(generation!=Volatile.Read(ref Generation))throw new HostError("card_changed",-61);
   args[0]=Token;readCode=read();
  }
  if(Wrapper!=null){Token=Wrapper.Token;OfflineToken=!String.IsNullOrEmpty(Token)&&Token.All(c=>c=='0');}
  LastFieldResults=(IDictionary)args[3];
  Diagnostic("read_result",new {sdk_code=readCode,sdk_name=SDKCodeName(readCode),fields=FieldResults(),state=Status(),wrapper=Wrapper!=null});
  Check(readCode);
  if(generation!=Volatile.Read(ref Generation))throw new HostError("card_changed",-61);
  if(Wrapper==null&&fields.Contains("C1")&&String.IsNullOrEmpty(Nested(args[2],"DateAdministrative","NumarAsigurat","Valoare"))){
   var auth=Str(Get(Session,"StareAutentificare"));
   if(auth=="AUTENTIFICAT"||auth=="AUTENTIFICAT_OFFLINE"||auth=="AUTENTIFICAT_OFFILINE")try{
    var cidList=(IList)Activator.CreateInstance(typeof(List<>).MakeGenericType(et));cidList.Add(Enum.Parse(et,"C1"));
    var cidResults=Activator.CreateInstance(results.GetType());object[] cidArgs={cidList,args[2],cidResults};
    var result=InvokePrivate(Get(Session,"TerminalManager"),"CitesteDate",cidArgs);
    Diagnostic("cnas_cid_read_retry",new {sdk_code=Convert.ToInt32(result)});
    if(Convert.ToInt32(result)==0){args[2]=cidArgs[1];foreach(DictionaryEntry entry in (IDictionary)cidArgs[2])LastFieldResults[entry.Key]=entry.Value;}
   }catch(Exception){Diagnostic("cnas_cid_read_retry_failed",null);}
  }
  if(generation!=Volatile.Read(ref Generation))throw new HostError("card_changed",-61);
  if(strict)foreach(DictionaryEntry entry in LastFieldResults)if(Convert.ToInt32(entry.Value)!=0)throw new HostError("field_read_failed",Convert.ToInt32(entry.Value));
  try{var certCode=Invoke(Session,"CitesteCertificate");Diagnostic("read_certificates_result",new {sdk_code=Convert.ToInt32(certCode),sdk_name=SDKCodeName(Convert.ToInt32(certCode))});}catch(Exception){Diagnostic("read_certificates_failed",null);}
  if(generation!=Volatile.Read(ref Generation))throw new HostError("card_changed",-61);
  return args[2];
 }
 static object FieldResults(){var result=new Dictionary<string,object>();if(LastFieldResults!=null)foreach(DictionaryEntry entry in LastFieldResults)result[Str(entry.Key)]=new {code=Convert.ToInt32(entry.Value),name=Str(entry.Value)};return result;}
 static object ReadResponse(string[] fields){
  var data=Read(fields,false);var projected=(Dictionary<string,object>)ProjectFields(data,fields);bool partial=false;
  foreach(var field in fields){var key=Enum.Parse(TypeNamed("CoduriCampuriCard"),field);if(!LastFieldResults.Contains(key)){partial=true;}else if(Convert.ToInt32(LastFieldResults[key])!=0){partial=true;projected.Remove(field);}}
  if(projected.Count==0&&fields.Length>0)throw new HostError("requested_fields_unavailable",0);
  projected["_field_results"]=FieldResults();projected["_partial"]=partial;projected["_offline_token"]=OfflineToken;
  return Result(projected);
 }
 static string Nested(object data,params string[] path){foreach(var p in path)data=Get(data,p);return Str(data);}
 static object Status(){var d=new Dictionary<string,object>();foreach(var n in new[]{"TerminalCurent","TerminalConectat","TerminalCuTastatura","StareAutentificare","StareCardInTerminal","NumarIncercariRamase","ProfilId","StareCard","StareComunicatieCuUM","NecesitaActualizare"})d[n]=Str(Get(Session,n));return d;}
 static void RequireEnrollment(Dictionary<string,object> contract,string host,int port){
  // Use the vendor's exact identity/hash implementation for every SDK version.
  var t=TypeNamed("TerminalDataIdentifier");var id=Activator.CreateInstance(t,true);
  Set(id,"CIF",Text(contract,"cif"));Set(id,"CUI",Text(contract,"cui"));Set(id,"NumarContract",Text(contract,"number"));Set(id,"DataContract",DateTime.ParseExact(Text(contract,"date"),"yyyy-MM-dd",System.Globalization.CultureInfo.InvariantCulture));Set(id,"CasaAsigurare",Text(contract,"house"));Set(id,"TipFurnizor",Text(contract,"provider_type"));Set(id,"NumeStatie",System.Net.Dns.GetHostName());
  var ipType=TypeNamed("IPAddressUM");var endpoint=ipType.GetProperty("Address",BindingFlags.Static|BindingFlags.Public|BindingFlags.NonPublic).GetValue(null,null);Set(id,"AdresaUM",endpoint.ToString());
  var storage=TypeNamed("TerminalDataStorage");var instance=storage.GetField("Instance",BindingFlags.Static|BindingFlags.Public|BindingFlags.NonPublic).GetValue(null);
  if(!(bool)Invoke(instance,"Exists",id))throw new HostError("enrollment_missing_use_explicit_enroll_or_migrate",-13);
  // Validate before ObtineToken: corrupt files must not trigger auto-enrollment.
  if(Invoke(instance,"Read",id)==null)throw new HostError("enrollment_invalid",-13);
 }
 static void AcquireToken(Dictionary<string,object> contract){
  TokenPrepared=false;
  // Detect the card model before token acquisition without subscribing to
  // insertion auto-authentication (important for the SDK's eID token path).
  var managerForTemplate=Get(Session,"TerminalManager");
  var ensureTemplate=managerForTemplate==null?null:managerForTemplate.GetType().GetMethod("EnsureCardTemplate",BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic,null,Type.EmptyTypes,null);
  if(ensureTemplate!=null)ensureTemplate.Invoke(managerForTemplate,null);
  Diagnostic("um_token_request_started",null);
  var rights=Activator.CreateInstance(TypeNamed("IdentificatorDrepturi"));
  Set(rights,"CUI",Text(contract,"cui"));Set(rights,"NumarContract",Text(contract,"number"));Set(rights,"DataContract",DateTime.ParseExact(Text(contract,"date"),"yyyy-MM-dd",System.Globalization.CultureInfo.InvariantCulture));Set(rights,"CasaAsigurare",Text(contract,"house"));Set(rights,"TipFurnizor",Text(contract,"provider_type"));OfflineToken=false;
  try{
   var acquired=Wrapper==null?Invoke(Session,"ObtineToken",Text(contract,"cif"),rights):Wrapper.AcquireToken();Token=acquired==null?null:Str(acquired);
   OfflineToken=Wrapper!=null&&!String.IsNullOrEmpty(Token)&&Token.All(c=>c=='0');
   if(OfflineToken)Diagnostic("cnas_offline_token",new {wrapper=true,profile=2});
  }catch(Exception ex){
   while(ex is TargetInvocationException&&ex.InnerException!=null)ex=ex.InnerException;
   var error=Classify(ex);Diagnostic("um_token_request_failed",new {operation=Text(Current,"operation"),sdk_code=error.SDKCode,sdk_name=SDKCodeName(error.SDKCode),host_version=HostVersion,wrapper=Wrapper!=null});
   var op=Text(Current,"operation");
   if(Wrapper!=null||(op!="read"&&op!="sign"&&op!="certificates")||!IsCode(error.SDKCode,"ERR_UM_INDISPONIBILA","ERR_UM_PROCESARE"))throw;
   var manager=Get(Session,"TerminalManager");var terminal=Get(manager,"TerminalData");var profiles=Get(terminal,"Profile") as IEnumerable;
   bool allowed=false;if(profiles!=null)foreach(var profile in profiles)if(Convert.ToInt32(Get(profile,"ProfilCard"))==2)allowed=true;
   if(!allowed||String.IsNullOrEmpty(Str(Get(terminal,"TerminalId"))))throw new HostError("offline_enrollment_profile_unavailable",error.SDKCode);
   var property=manager.GetType().GetProperty("ProfilId",BindingFlags.Instance|BindingFlags.NonPublic|BindingFlags.Public);
   if(property==null||!property.CanWrite)throw new HostError("sdk_incompatible",0);
   property.SetValue(manager,2,null);Token=new string('0',32);Set(Session,"Token",Token);OfflineToken=true;
   Diagnostic("cnas_offline_token",new {wrapper=false,sdk_code=error.SDKCode,profile=2});
  }
  TokenPrepared=true;
  Diagnostic("um_token_request_finished",new {received=!String.IsNullOrEmpty(Token),offline=OfflineToken,state=Str(Get(Session,"StareComunicatieCuUM"))});
 }
 static string[] Fields(Dictionary<string,object> req,string[] defaults){object f;if(!req.TryGetValue("fields",out f))return defaults;return ((IEnumerable)f).Cast<object>().Select(Str).ToArray();}
 static void CheckIdentity(Dictionary<string,object> expected,object card){
  var cnp=Nested(card,"DateIdentificare","CNP","Valoare").Trim();var cid=Nested(card,"DateAdministrative","NumarAsigurat","Valoare").Trim();var ec=Text(expected,"cnp").Trim();var ei=Text(expected,"cid").Trim();
  bool anonymous=cnp==""||cnp=="0000000000000";
  if(anonymous){if(ei==""||cid==""||ei!=cid)throw new HostError("matching_cid_required",0);}
  else {if(ec!=""&&ec!="0000000000000"&&ec!=cnp)throw new HostError("cnp_mismatch",0);if(ei!=""&&ei!=cid)throw new HostError("cid_mismatch",0);if((ec==""||ec=="0000000000000")&&ei=="")throw new HostError("expected_identity_required",0);}
 }
 // PC/SC enumerates attached devices; the SDK catalogue is not hardware detection.
 [DllImport("winscard.dll")] static extern int SCardEstablishContext(uint scope,IntPtr r1,IntPtr r2,out IntPtr context);
 [DllImport("winscard.dll",CharSet=CharSet.Unicode,ExactSpelling=true)] static extern int SCardListReadersW(IntPtr context,string groups,IntPtr readers,ref uint length);
 [DllImport("winscard.dll")] static extern int SCardReleaseContext(IntPtr context);
 static string[] ConnectedReaders(){
  IntPtr context;int rc=SCardEstablishContext(2,IntPtr.Zero,IntPtr.Zero,out context);
  if(rc!=0)throw new HostError("pcsc_context_failed",rc);
  try {
   for(int attempt=0;attempt<3;attempt++){
    uint length=0;rc=SCardListReadersW(context,null,IntPtr.Zero,ref length);
    if(rc==unchecked((int)0x8010002E))return new string[0];
    if(rc!=0)throw new HostError("pcsc_list_readers_failed",rc);
    if(length==0)return new string[0];
    if(length>1048576)throw new HostError("pcsc_reader_list_too_large",0);
    IntPtr buffer=Marshal.AllocHGlobal(checked((int)length*2));
    try {
     rc=SCardListReadersW(context,null,buffer,ref length);
     if(rc==unchecked((int)0x80100008))continue; // Device list changed: retry sizing.
     if(rc==unchecked((int)0x8010002E))return new string[0];
     if(rc!=0)throw new HostError("pcsc_list_readers_failed",rc);
     return Marshal.PtrToStringUni(buffer,(int)length).Split(new[]{'\0'},StringSplitOptions.RemoveEmptyEntries).Distinct().ToArray();
    } finally {Marshal.FreeHGlobal(buffer);}
   }
   throw new HostError("pcsc_reader_list_changed_retry",0);
  } finally {SCardReleaseContext(context);}
 }
 static string ChooseReader(string selected,string[] supported,string[] connected){
  var available=connected.Where(x=>supported.Any(model=>x.Equals(model,StringComparison.OrdinalIgnoreCase)||x.StartsWith(model+" ",StringComparison.OrdinalIgnoreCase))).ToArray();
  if(selected==""){if(available.Length==1)return available[0];if(available.Length==0)throw new HostError("no_supported_reader_connected",0);throw new HostError("select_physical_reader",0);}
  if(available.Contains(selected))return selected;
  var matching=available.Where(x=>x.Equals(selected,StringComparison.OrdinalIgnoreCase)||x.StartsWith(selected+" ",StringComparison.OrdinalIgnoreCase)).ToArray();
  if(supported.Contains(selected)&&matching.Length==1)return matching[0];
  if(matching.Length>1)throw new HostError("select_physical_reader",0);
  throw new HostError("selected_reader_not_connected_or_supported",0);
 }
 static string SDKReader(Type manager,string selected){
  var supported=((IEnumerable)Invoke(manager,"GetSupportedTerminalNames")).Cast<object>().Select(Str).ToArray();
  return ChooseReader(selected,supported,ConnectedReaders());
 }
 static void HookCardInitialization(){
  CardInitializationCode=0;
  if(Session.GetType().GetEvent("DupaStareCardInTerminalSchimbata")==null)return;
  Hook("DupaStareCardInTerminalSchimbata",args=>{
   if(args.Length>=2 && Str(args[0])=="CardInserat")Interlocked.Exchange(ref CardInitializationCode,Convert.ToInt32(args[1]));
   Diagnostic("DupaStareCardInTerminalSchimbata",new {states=args.Where(x=>x!=null).Select(Str).ToArray()});
  });
 }
 static void AttachPreparedCardNotifications(){
  if(!TokenPrepared)throw new HostError("token_preparation_incomplete",0);
  if(CardNotificationsAttached)return;
  // Adding this handler itself can queue SDK auto-authentication. Do not do
  // that until AcquireToken has finished setting terminal data and ProfilId.
  Hook("StareCardInTerminalSchimbata",args=>{Interlocked.Increment(ref Generation);Diagnostic("card_state",new {state=Str(Get(Session,"StareCardInTerminal"))});});
  CardNotificationsAttached=true;
  Diagnostic("card_notifications_attached",new {token_prepared=TokenPrepared,profile=Str(Get(Get(Session,"TerminalManager"),"ProfilId"))});
 }
 static void CompleteCardInitialization(){
  // This SDK entry point shares its own card-state lock with the asynchronous
  // insertion callback. Calling it drains pending initialization (or performs
  // it once); the SDK suppresses duplicate insertion notifications itself.
  var method=Session.GetType().GetMethods().SingleOrDefault(m=>m.Name=="SchimbareStareCardInTerminal"&&m.GetParameters().Length==1&&m.GetParameters()[0].ParameterType.IsEnum);
  if(method==null){Diagnostic("card_initialization_sync_unavailable",null);return;}
  Stage="card_initialization";Diagnostic("card_initialization_wait",null);
  method.Invoke(Session,new[]{Enum.Parse(method.GetParameters()[0].ParameterType,"CardInserat")});
  int code=Volatile.Read(ref CardInitializationCode);
  Diagnostic("card_initialization_finished",new {sdk_code=code,state=Status()});
  // An offline authentication state alone does not prove authentication
  // succeeded: the vendor can set that state before returning a negative code.
  if(new[]{-11,-14,-15,-28,-40,-41,-42,-52}.Contains(code)){
   if(Wrapper!=null){int initializationCode=code;code=Wrapper.Evaluate(()=>initializationCode);}
   Check(code);
  }
 }
 static void WaitForReader(bool requireCard){
  var until=DateTime.UtcNow.AddSeconds(5);Diagnostic("reader_initialization_wait",new {require_card=requireCard});
  do{if(Convert.ToBoolean(Get(Session,"TerminalConectat")??false)&&(!requireCard||Str(Get(Session,"StareCardInTerminal"))=="CardInserat")){Diagnostic("reader_ready",Status());return;}Thread.Sleep(100);}while(DateTime.UtcNow<until);
  throw new HostError(requireCard?"card_not_ready":"reader_not_ready",0);
 }
 static object Open(Dictionary<string,object> input){
  if(Opened)throw new HostError("session_already_open",0);Opened=true;Current=Dict(input,"request");var op=Text(Current,"operation");Diagnostic("sdk_loaded",SDKIdentity());if(op=="readers")return Result(ConnectedReaders());var manager=TypeNamed("ManagerSesiuniCard");string um=Text(input,"um_host");int port=Convert.ToInt32(input["um_port"]);Invoke(manager,"SetAdresaUnitateManagement",um,port);
  var physicalReader=SDKReader(manager,Text(input,"reader"));Diagnostic("reader_selected",new {reader=physicalReader});
  Session=Invoke(manager,"StartSesiuneNoua",physicalReader);
  Hook("UserInputRequired",UserInput);
  HookCardInitialization();
  if(WrapperMode){Stage="cnas_wrapper_initialize";Wrapper=new CnasWrapperAdapter(Path.Combine(Path.GetDirectoryName(SDK.Location),"Cnas.Ceas.SmartCard.Novensys.dll"),Session,Dict(Current,"contract"),Diagnostic);}
  if(!WrapperMode)ConfigureCNASOfflineResponses();
  Hook("StareEditarePeTerminalSchimbata",args=>{if(Convert.ToInt32(args[0])!=0)PinAttempted=true;Diagnostic("terminal_input_state",new {state=Str(args[0])});});
  foreach(var eventName in new[]{"StareCardSchimbata","StareComunicatieCuUMSchimbata","StareAutentificareSchimbata"})HookDiagnostic(eventName);
  Diagnostic("session_state",Status());
  if(op=="status")return Result(Status());
  WaitForReader(op!="enroll");
  if(op=="enroll")return Result(new {ready=true});
  var contract=Dict(Current,"contract");RequireEnrollment(contract,um,port);AcquireToken(contract);
  AttachPreparedCardNotifications();
  if(op=="read"||op=="sign"||op=="certificates")CompleteCardInitialization();
  if(op=="write"||op=="activate"||op=="change-pin"||op=="reset-pin")return Result(new {ready=true});
  if(op=="certificates"){Check(Invoke(Session,"CitesteCertificate"));var certs=new Dictionary<string,string>();foreach(var p in Session.GetType().GetProperties()){if(typeof(X509Certificate).IsAssignableFrom(p.PropertyType)){var c=p.GetValue(Session,null) as X509Certificate;if(c!=null)certs[p.Name]=Convert.ToBase64String(c.GetRawCertData());}}var tm=Get(Session,"TerminalManager");foreach(var name in new[]{"Certificat","CertificatMAI"}){var cert=Get(tm,name) as X509Certificate;if(cert!=null)certs[name]=Convert.ToBase64String(cert.GetRawCertData());}return Result(certs);}
  if(op=="read"){var fields=Fields(Current,new[]{"A1","B1","B2","B4","C1"});return ReadResponse(fields);}
  if(op=="sign"){
   int before=Volatile.Read(ref Generation);var card=Read(new[]{"A1","B4","C1"});CheckIdentity(Dict(Current,"expected"),card);
   if(before!=Volatile.Read(ref Generation))throw new HostError("card_changed_during_identity_read",-61);
   PinnedGeneration=before;CertificateHash=CardFingerprint();PinnedCertificate=new X509Certificate2(SigningCertificate());
   return Result(new {cnp=Nested(card,"DateIdentificare","CNP","Valoare"),cid=Nested(card,"DateAdministrative","NumarAsigurat","Valoare")});
  }
  throw new HostError("unsupported_operation",0);
 }
 static X509Certificate SigningCertificate(){
  var manager=Get(Session,"TerminalManager");bool mai=Text(Current,"certificate")=="mai";
  var cert=Get(manager,mai?"CertificatMAI":"Certificat") as X509Certificate;
  if(cert!=null)return cert;
  // New SDK's CEI certificates are also available in SetAllCerts. Older
  // versions do not have this method; missing capabilities fail explicitly.
  var method=manager.GetType().GetMethod("SetAllCerts",BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic);
  if(method!=null){var certs=method.Invoke(manager,new object[]{null}) as IDictionary;if(certs!=null)cert=certs[mai?"CertificatMAI":"CertificatAsigurat"] as X509Certificate;}
  return cert;
 }
 static string CardFingerprint(){
  // Reread certificate through SDK and compare raw DER; never rely on CNP alone.
  Check(Invoke(Session,"CitesteCertificate"));var c=SigningCertificate();
  if(c==null)throw new HostError("certificate_unavailable",-51);
  using(var sha=SHA256.Create())return Convert.ToBase64String(sha.ComputeHash(c.GetRawCertData()));
 }
 static void CheckPinnedCard(){if(PinnedGeneration!=Volatile.Read(ref Generation))throw new HostError("card_changed",-61);if(CardFingerprint()!=CertificateHash)throw new HostError("card_changed",-61);if(PinnedGeneration!=Volatile.Read(ref Generation))throw new HostError("card_changed",-61);}
 static object Sign(Dictionary<string,object> input){
  if(Text(Current,"operation")!="sign")throw new HostError("not_a_signing_session",0);CheckPinnedCard();var text=Text(input,"text");var encoding=Text(input,"encoding");byte[] bytes;
  if(encoding=="ascii"){if(text.Any(c=>c>127))throw new HostError("non_ascii_text",0);bytes=Encoding.ASCII.GetBytes(text);}else if(encoding=="utf-8"){bytes=new UTF8Encoding(false,true).GetBytes(text);}else throw new HostError("invalid_encoding",0);
  var method=Text(input,"certificate")=="mai"?"ComputeHashMAI":"ComputeHash";
  if(Wrapper!=null&&method=="ComputeHashMAI")throw new HostError("sdk_incompatible",0);
  byte[] signed=Wrapper==null?(byte[])Invoke(Session,method,bytes):Wrapper.Sign(bytes);CheckPinnedCard();
  var cms=new SignedCms();cms.Decode(signed);if(!cms.ContentInfo.Content.SequenceEqual(bytes)||cms.SignerInfos.Count!=1)throw new HostError("signature_content_mismatch",0);
  // Verify using ONLY the certificate captured from the identified card. Do not
  // accept a replacement signer's certificate embedded in the CMS response.
  var signer=cms.SignerInfos[0];signer.CheckSignature(new X509Certificate2Collection(PinnedCertificate),true);
  if(signer.SignerIdentifier.Type==SubjectIdentifierType.IssuerAndSerialNumber){var issuer=(System.Security.Cryptography.Xml.X509IssuerSerial)signer.SignerIdentifier.Value;if(!String.Equals(issuer.SerialNumber.TrimStart('0'),PinnedCertificate.SerialNumber.TrimStart('0'),StringComparison.OrdinalIgnoreCase))throw new HostError("signer_mismatch",0);}
  if(signer.Certificate!=null&&!signer.Certificate.RawData.SequenceEqual(PinnedCertificate.RawData))throw new HostError("signer_mismatch",0);
  return Result(Convert.ToBase64String(signed));
 }
 static object ProjectFields(object data,string[] requested){
  var result=new Dictionary<string,object>();
  foreach(var group in data.GetType().GetProperties()){
   var obj=group.GetValue(data,null);if(obj==null)continue;
   foreach(var p in obj.GetType().GetProperties()){
    if(p.GetIndexParameters().Length!=0)continue;var field=p.GetValue(obj,null);if(field==null||field.GetType().Name!="Camp")continue;
    var code=Str(Get(field,"Cod"));if(requested.Contains(code)&&code!="G2"&&code!="G3"&&code!="G4")result[code]=Get(field,"Valoare");
   }
  }
  return result;
 }
 static void Populate(object target,Dictionary<string,object> values){
  foreach(var entry in values){var p=target.GetType().GetProperty(entry.Key);if(p==null)throw new HostError("unknown_data_property",0);var nested=entry.Value as Dictionary<string,object>;if(nested!=null){var child=p.GetValue(target,null);if(child==null)throw new HostError("invalid_nested_data",0);Populate(child,nested);}else {if(!p.CanWrite)throw new HostError("readonly_data_property",0);Type valueType=p.PropertyType;
   if(p.Name=="Valoare"&&target.GetType().Name=="Camp") {var fieldCode=Str(Get(target,"Cod"));if(fieldCode=="D6"||fieldCode=="D7")valueType=typeof(List<string>);else if(fieldCode=="C8")valueType=typeof(List<>).MakeGenericType(TypeNamed("PersoanaContact"));}
   p.SetValue(target,Json.Deserialize(Json.Serialize(entry.Value),valueType),null);}}
 }
 static object Execute(){var op=Text(Current,"operation");if(op=="enroll"){AcquireToken(Dict(Current,"contract"));if(Str(Get(Session,"TerminalId"))=="")throw new HostError("enrollment_failed",-13);return Result(new {enrolled=true});}
  if(op=="write"){
   var fields=Fields(Current,new string[0]);
   var et=TypeNamed("CoduriCampuriCard");var list=(IList)Activator.CreateInstance(typeof(List<>).MakeGenericType(et));foreach(var f in fields)list.Add(Enum.Parse(et,f));var data=Activator.CreateInstance(TypeNamed("CardData"));Populate(data,Dict(Current,"data"));var results=Activator.CreateInstance(typeof(Dictionary<,>).MakeGenericType(et,TypeNamed("CoduriRaspunsOperatieCamp")));object[] args={Token,Text(Current,"cid"),list,data,results};Check(Wrapper==null?Convert.ToInt32(Invoke(Session,"EditeazaDate",args)):Wrapper.Evaluate(()=>Convert.ToInt32(Invoke(Session,"EditeazaDate",args))));return Result(args[4]);
  }
  if(Wrapper!=null){Wrapper.Execute(op);return Result(new {completed=true,wrapper=true});}
  string method=op=="activate"?"ActiveazaCard":op=="change-pin"?"SchimbaPIN":op=="reset-pin"?"ReseteazaPIN":null;if(method==null)throw new HostError("unsupported_operation",0);Check(Invoke(Session,method,Token));return Result(new {completed=true});
 }
 // The CNAS desktop entrypoint is STA. Keep its wrapper on the same apartment
 // model, without changing the already-working direct SDK execution path.
 public static int Main(string[] args){
  if(args.Contains("--cnas-wrapper")&&Thread.CurrentThread.GetApartmentState()!=ApartmentState.STA){
   int result=1;Exception failure=null;object details=null;
   var thread=new Thread(()=>{try{result=RunMain(args);}catch(Exception ex){failure=ex;details=ExceptionDiagnostics(ex);}});
   thread.SetApartmentState(ApartmentState.STA);thread.Start();thread.Join();
   if(failure!=null){Send(new {ok=false,code="sdk_exception",exception_type=failure.GetBaseException().GetType().FullName,stage="cnas_wrapper_initialize",diagnostics=details,host_version=HostVersion});}
   return result;
  }
  return RunMain(args);
 }
 static object ExceptionDiagnostics(Exception exception){
  var root=exception.GetBaseException();var native=root as Win32Exception;
  if(native==null&&Stage!="cnas_wrapper_initialize")return null;
  var chain=new List<object>();var names=new string[0];int depth=0;
  for(var cause=exception;cause!=null&&depth++<16;cause=cause.InnerException){
   var frames=new StackTrace(cause,false).GetFrames();
   var current=frames==null?new string[0]:frames.Select(frame=>frame.GetMethod()).Where(method=>method!=null).Take(16).Select(method=>(method.DeclaringType==null?"":method.DeclaringType.FullName+".")+method.Name).ToArray();
   if(current.Length>0)names=current;
   var initializer=cause as TypeInitializationException;
   // Constructor diagnostics contain no PIN/token or request arguments. Keep
   // the wrapper message: the innermost Win32Exception can be an unthrown inner
   // cause with no stack, while the outer exception identifies the real failure.
   var message=Stage=="cnas_wrapper_initialize"?cause.Message:null;
   if(message!=null&&message.Length>2048)message=message.Substring(0,2048);
   chain.Add(new {exception_type=cause.GetType().FullName,type_name=initializer==null?null:initializer.TypeName,message=message,hresult=cause.HResult,stack_frames=current});
  }
  return new {native_error_code=native==null?(int?)null:native.NativeErrorCode,native_message=native==null?null:native.Message,initialization_step=WrapperMode?CnasWrapperAdapter.InitializationStep:null,apartment_state=Thread.CurrentThread.GetApartmentState().ToString(),stack_frames=names,exception_chain=chain};
 }
 static int RunMain(string[] args){
  Output=Console.Out;if(args.Length==1&&args[0]=="--version"){Send(new {host_version=HostVersion});return 0;}Console.SetOut(TextWriter.Null);Console.SetError(TextWriter.Null);
  var lease=new Mutex(false,@"Global\WiseMED.PIAS.eCard");bool acquired=false;
  try{acquired=lease.WaitOne(0);}catch(AbandonedMutexException){acquired=true;}
  if(!acquired){Send(new{ok=false,code="reader_busy"});lease.Dispose();return 2;}
  try{
   WrapperMode=args.Length>1&&args[1]=="--cnas-wrapper";
   string dll=Path.GetFullPath(args[0]);AppDomain.CurrentDomain.AssemblyResolve+=(sender,e)=>{string path=Path.Combine(Path.GetDirectoryName(dll),new AssemblyName(e.Name).Name+".dll");return File.Exists(path)?Assembly.LoadFrom(path):null;};SDK=Assembly.LoadFrom(dll);using(var hash=SHA256.Create()){SDKFingerprint=BitConverter.ToString(hash.ComputeHash(File.ReadAllBytes(dll))).Replace("-","").ToLowerInvariant();}System.Runtime.CompilerServices.RuntimeHelpers.RunModuleConstructor(SDK.ManifestModule.ModuleHandle);DisableSDKLogs();
   string line;while((line=Console.ReadLine())!=null){try{ActiveCommand=true;var request=Json.Deserialize<Dictionary<string,object>>(line);var action=Text(request,"action");if(action=="open")Send(Open(request));else if(action=="sign")Send(Sign(request));else if(action=="execute")Send(Execute());else throw new HostError("unknown_action",0);}
    catch(Exception ex){var diagnostics=ExceptionDiagnostics(ex);while(ex is TargetInvocationException&&ex.InnerException!=null)ex=ex.InnerException;var h=Classify(ex);var native=ex.GetBaseException() as Win32Exception;Send(new {ok=false,exception_type=ex.GetBaseException().GetType().FullName,diagnostics=diagnostics,code=h.Code,sdk_code=h.SDKCode,message=native==null?ErrorText(h):native.Message,host_version=HostVersion,sdk_identity=SDKIdentity(),sdk_name=h.Code=="field_read_failed"||h.SDKCode==0?"":SDKCodeName(h.SDKCode),data=new {field_results=FieldResults()},stage=Stage,state=Session==null?null:Status(),pin_attempted=PinAttempted});}finally{ActiveCommand=false;}
   }
  }catch(Exception){Send(new {ok=false,code="sdk_unavailable"});return 1;}
  finally{if(Session!=null)try{Invoke(Session,"Stop");}catch{}if(acquired)lease.ReleaseMutex();lease.Dispose();}
  return 0;
 }
 static string ErrorText(HostError error){
  if(error.Code=="selected_reader_not_connected_or_supported")return "Terminalul de citire carduri selectat nu este detectat";
  if(error.Code=="field_read_failed")return "Un câmp solicitat nu a putut fi citit. Verificați rezultatele pe câmpuri.";
  if(error.Code=="sdk_error")try{var messages=Activator.CreateInstance(TypeNamed("MesajeRaspunsCard")) as IDictionary;var key=Enum.ToObject(TypeNamed("CoduriRaspunsOperatieCard"),error.SDKCode);if(messages!=null&&messages.Contains(key))return Str(messages[key]);}catch{}
  return null;
 }
 static string SDKCodeName(int code){try{return Enum.GetName(TypeNamed("CoduriRaspunsOperatieCard"),code)??"";}catch{return "";}}
 static HostError Classify(Exception ex){
  var known=ex as HostError;if(known!=null)return known;
  if(ex is TypeLoadException||ex is MissingMethodException||ex is ReflectionTypeLoadException)return new HostError("sdk_incompatible",0);
  if(ex is FileNotFoundException||ex is FileLoadException||ex is BadImageFormatException)return new HostError("sdk_unavailable",0);
  if(Wrapper!=null){var code=Wrapper.SDKError(ex);if(code.HasValue&&code.Value!=0)return new HostError("sdk_error",code.Value);}
  try{var messages=Activator.CreateInstance(TypeNamed("MesajeRaspunsCard")) as IDictionary;if(messages!=null)for(var cause=ex;cause!=null;cause=cause.InnerException)foreach(DictionaryEntry entry in messages)if(Str(entry.Value)==cause.Message)return new HostError("sdk_error",Convert.ToInt32(entry.Key));}catch{}
  return new HostError("sdk_exception",0);
 }
 sealed class HostError:Exception {public readonly string Code;public readonly int SDKCode;public HostError(string code,int n){Code=code;SDKCode=n;}}
}
