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
using System.Web.Script.Serialization;

public static class ECardHost {
 static readonly JavaScriptSerializer Json = new JavaScriptSerializer { MaxJsonLength = 2 * 1024 * 1024, RecursionLimit = 64 };
 static TextWriter Output;
 static Assembly SDK;
 static object Session;
 static string Token;
 static int Generation;
 static int PinnedGeneration;
 static bool PinAttempted;
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
 static object Invoke(object obj,string name,params object[] args){var t=obj as Type ?? obj.GetType();var methods=t.GetMethods().Where(m=>m.Name==name&&m.GetParameters().Length==args.Length).ToArray();if(methods.Length!=1)throw new HostError("sdk_incompatible",0);return methods[0].Invoke(obj is Type?null:obj,args);}
 static void Check(object code){int n=Convert.ToInt32(code);if(n!=0)throw new HostError("sdk_error",n);}
 static void Send(object result){lock(Output){Output.WriteLine(Json.Serialize(result));Output.Flush();}}
 static object Result(object data){return new {ok=true,data=data,pin_attempted=PinAttempted};}
 static void Hook(string name,Action<object[]> handler){
  var ev=Session.GetType().GetEvent(name);if(ev==null)throw new HostError("sdk_incompatible",0);
  var pars=ev.EventHandlerType.GetMethod("Invoke").GetParameters().Select(p=>Expression.Parameter(p.ParameterType,p.Name)).ToArray();
  var array=Expression.NewArrayInit(typeof(object),pars.Select(p=>Expression.Convert(p,typeof(object))));
  var body=Expression.Call(Expression.Constant(handler),typeof(Action<object[]>).GetMethod("Invoke"),array);
  var del=Expression.Lambda(ev.EventHandlerType,body,pars).Compile();ev.AddEventHandler(Session,del);Hooks.Add(del);
 }
 static void UserInput(object[] args){
  lock(InputLock){var e=args[args.Length-1];if(!ActiveCommand){Set(e,"Cancel",true);return;}PinAttempted=true;Send(new { @event="pin", prompt=Str(Get(e,"Prompt")) });
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
 static object Read(string[] fields){
  var et=TypeNamed("CoduriCampuriCard");var list=(IList)Activator.CreateInstance(typeof(List<>).MakeGenericType(et));
  foreach(var f in fields){if(!Enum.IsDefined(et,f))throw new HostError("sdk_incompatible",0);list.Add(Enum.Parse(et,f));}
  var data=Activator.CreateInstance(TypeNamed("CardData"));var results=Activator.CreateInstance(typeof(Dictionary<,>).MakeGenericType(et,TypeNamed("CoduriRaspunsOperatieCamp")));
  object[] args={Token,list,data,results};Check(Invoke(Session,"CitesteDate",args));
  foreach(DictionaryEntry entry in (IDictionary)args[3])if(Convert.ToInt32(entry.Value)!=0)throw new HostError("field_read_failed",Convert.ToInt32(entry.Value));
  return args[2];
 }
 static string Nested(object data,params string[] path){foreach(var p in path)data=Get(data,p);return Str(data);}
 static object Status(){var d=new Dictionary<string,object>();foreach(var n in new[]{"TerminalCurent","TerminalConectat","TerminalCuTastatura","StareAutentificare","StareCardInTerminal","NumarIncercariRamase","StareCard","StareComunicatieCuUM","NecesitaActualizare"})d[n]=Str(Get(Session,n));return d;}
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
  var rights=Activator.CreateInstance(TypeNamed("IdentificatorDrepturi"));
  Set(rights,"CUI",Text(contract,"cui"));Set(rights,"NumarContract",Text(contract,"number"));Set(rights,"DataContract",DateTime.ParseExact(Text(contract,"date"),"yyyy-MM-dd",System.Globalization.CultureInfo.InvariantCulture));Set(rights,"CasaAsigurare",Text(contract,"house"));Set(rights,"TipFurnizor",Text(contract,"provider_type"));Token=Str(Invoke(Session,"ObtineToken",Text(contract,"cif"),rights));
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
 static string SDKReader(Type manager,string selected){
  if(selected=="")return selected;
  var supported=((IEnumerable)Invoke(manager,"GetSupportedTerminalNames")).Cast<object>().Select(Str).ToArray();
  // Legacy saved configurations contain SDK model names. Preserve their semantics.
  if(supported.Contains(selected))return selected;
  var connected=ConnectedReaders();
  if(!connected.Contains(selected))throw new HostError("selected_reader_not_connected",0);
  var model=supported.OrderByDescending(x=>x.Length).FirstOrDefault(x=>selected.StartsWith(x+" ",StringComparison.OrdinalIgnoreCase));
  if(model==null)throw new HostError("selected_reader_not_supported_by_sdk",0);
  // SDK selects a model, not a PC/SC instance. Never silently use another twin.
  if(connected.Count(x=>x.Equals(model,StringComparison.OrdinalIgnoreCase)||x.StartsWith(model+" ",StringComparison.OrdinalIgnoreCase))>1)throw new HostError("multiple_readers_of_same_model_not_supported_by_sdk",0);
  return model;
 }
 static object Open(Dictionary<string,object> input){
  if(Opened)throw new HostError("session_already_open",0);Opened=true;Current=Dict(input,"request");var op=Text(Current,"operation");if(op=="readers")return Result(ConnectedReaders());var manager=TypeNamed("ManagerSesiuniCard");string um=Text(input,"um_host");int port=Convert.ToInt32(input["um_port"]);Invoke(manager,"SetAdresaUnitateManagement",um,port);
  Session=Invoke(manager,"StartSesiuneNoua",SDKReader(manager,Text(input,"reader")));
  Hook("UserInputRequired",UserInput);
  Hook("StareCardInTerminalSchimbata",args=>Interlocked.Increment(ref Generation));
  Hook("StareEditarePeTerminalSchimbata",args=>{if(Convert.ToInt32(args[0])!=0)PinAttempted=true;});
  if(op=="status")return Result(Status());
  if(op=="enroll")return Result(new {ready=true});
  var contract=Dict(Current,"contract");RequireEnrollment(contract,um,port);AcquireToken(contract);
  if(op=="write"||op=="activate"||op=="change-pin"||op=="reset-pin")return Result(new {ready=true});
  if(op=="certificates"){Check(Invoke(Session,"CitesteCertificate"));var certs=new Dictionary<string,string>();foreach(var p in Session.GetType().GetProperties()){if(typeof(X509Certificate).IsAssignableFrom(p.PropertyType)){var c=p.GetValue(Session,null) as X509Certificate;if(c!=null)certs[p.Name]=Convert.ToBase64String(c.GetRawCertData());}}var tm=Get(Session,"TerminalManager");foreach(var name in new[]{"Certificat","CertificatMAI"}){var cert=Get(tm,name) as X509Certificate;if(cert!=null)certs[name]=Convert.ToBase64String(cert.GetRawCertData());}return Result(certs);}
  if(op=="read"){var fields=Fields(Current,new[]{"A1","B1","B2","B4","C1"});return Result(ProjectFields(Read(fields),fields));}
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
  byte[] signed=(byte[])Invoke(Session,method,bytes);CheckPinnedCard();
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
   var fields=Fields(Current,new string[0]);var et=TypeNamed("CoduriCampuriCard");var list=(IList)Activator.CreateInstance(typeof(List<>).MakeGenericType(et));foreach(var f in fields)list.Add(Enum.Parse(et,f));var data=Activator.CreateInstance(TypeNamed("CardData"));Populate(data,Dict(Current,"data"));var results=Activator.CreateInstance(typeof(Dictionary<,>).MakeGenericType(et,TypeNamed("CoduriRaspunsOperatieCamp")));object[] args={Token,Text(Current,"cid"),list,data,results};Check(Invoke(Session,"EditeazaDate",args));return Result(args[4]);
  }
  string method=op=="activate"?"ActiveazaCard":op=="change-pin"?"SchimbaPIN":op=="reset-pin"?"ReseteazaPIN":null;if(method==null)throw new HostError("unsupported_operation",0);Check(Invoke(Session,method,Token));return Result(new {completed=true});
 }
 public static int Main(string[] args){
  Output=Console.Out;Console.SetOut(TextWriter.Null);Console.SetError(TextWriter.Null);
  var lease=new Mutex(false,@"Global\WiseMED.PIAS.eCard");bool acquired=false;
  try{acquired=lease.WaitOne(0);}catch(AbandonedMutexException){acquired=true;}
  if(!acquired){Send(new{ok=false,code="reader_busy"});lease.Dispose();return 2;}
  try{
   string dll=Path.GetFullPath(args[0]);AppDomain.CurrentDomain.AssemblyResolve+=(sender,e)=>{string path=Path.Combine(Path.GetDirectoryName(dll),new AssemblyName(e.Name).Name+".dll");return File.Exists(path)?Assembly.LoadFrom(path):null;};SDK=Assembly.LoadFrom(dll);System.Runtime.CompilerServices.RuntimeHelpers.RunModuleConstructor(SDK.ManifestModule.ModuleHandle);DisableSDKLogs();
   string line;while((line=Console.ReadLine())!=null){try{ActiveCommand=true;var request=Json.Deserialize<Dictionary<string,object>>(line);var action=Text(request,"action");if(action=="open")Send(Open(request));else if(action=="sign")Send(Sign(request));else if(action=="execute")Send(Execute());else throw new HostError("unknown_action",0);}
    catch(Exception ex){while(ex is TargetInvocationException&&ex.InnerException!=null)ex=ex.InnerException;var h=Classify(ex);Send(new {ok=false,code=h.Code,sdk_code=h.SDKCode,pin_attempted=PinAttempted});}finally{ActiveCommand=false;}
   }
  }catch(Exception){Send(new {ok=false,code="sdk_unavailable"});return 1;}
  finally{if(Session!=null)try{Invoke(Session,"Stop");}catch{}if(acquired)lease.ReleaseMutex();lease.Dispose();}
  return 0;
 }
 static HostError Classify(Exception ex){
  var known=ex as HostError;if(known!=null)return known;
  if(ex is TypeLoadException||ex is MissingMethodException||ex is ReflectionTypeLoadException)return new HostError("sdk_incompatible",0);
  if(ex is FileNotFoundException||ex is FileLoadException||ex is BadImageFormatException)return new HostError("sdk_unavailable",0);
  try{var messages=Activator.CreateInstance(TypeNamed("MesajeRaspunsCard")) as IDictionary;if(messages!=null)foreach(DictionaryEntry entry in messages)if(Str(entry.Value)==ex.Message)return new HostError("sdk_error",Convert.ToInt32(entry.Key));}catch{}
  return new HostError("sdk_exception",0);
 }
 sealed class HostError:Exception {public readonly string Code;public readonly int SDKCode;public HostError(string code,int n){Code=code;SDKCode=n;}}
}
