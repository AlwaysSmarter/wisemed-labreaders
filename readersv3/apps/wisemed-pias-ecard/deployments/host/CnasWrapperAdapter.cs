// Loads the original CNAS wrapper. No decompiled vendor source is compiled here.
// Its desktop persistence/UI are replaced only at the context boundary: the
// current SDK session and contractual identity are supplied by the WiseMED job.
using System;
using System.Collections.Generic;
using System.Linq;
using System.Reflection;
using System.Reflection.Emit;

internal sealed class CnasWrapperAdapter {
 readonly object context;
 readonly object provider;
 readonly object credentials;
 readonly Type contextType;
 readonly Action<string,object> diagnostic;
 public string Version {get;private set;}
 public string SHA256 {get;private set;}
 public static string InitializationStep {get;private set;}
 const BindingFlags Members=BindingFlags.Instance|BindingFlags.NonPublic|BindingFlags.Public;

 public CnasWrapperAdapter(string path,object session,Dictionary<string,object> contract,Action<string,object> sink){
  diagnostic=sink;
  InitializeStep("load_assembly");
  var assembly=Assembly.LoadFrom(path);Version=assembly.GetName().Version.ToString();
  using(var hash=System.Security.Cryptography.SHA256.Create()){SHA256=BitConverter.ToString(hash.ComputeHash(System.IO.File.ReadAllBytes(path))).Replace("-","").ToLowerInvariant();}
  InitializeStep("resolve_context_type");
  contextType=assembly.GetType("Cnas.Ceas.SmartCard.Novensys.Services.NovensysSmartCardContext",true);
  var providerMethod=contextType.GetMethod("GetProviderData");
  // The first field initializer of the vendor context accesses Host.Container.
  // Initialize that same vendor dependency explicitly so a failing .cctor is
  // reported separately, rather than hidden inside context construction.
  var shellReference=assembly.GetReferencedAssemblies().FirstOrDefault(a=>a.Name=="Simavi.Applications.Shell");
  if(shellReference!=null){
   InitializeStep("initialize_cnas_shell");
   var shell=Assembly.Load(shellReference);
   var host=shell.GetType("Simavi.Applications.Shell.Host",true);
   System.Runtime.CompilerServices.RuntimeHelpers.RunClassConstructor(host.TypeHandle);
   InitializeStep("register_standalone_credentials");
   var credentialInterface=contextType.GetMethod("GetToken",Members).GetParameters()[0].ParameterType;
   credentials=RegisterStandaloneCredentials(host.GetProperty("Container",BindingFlags.Static|BindingFlags.Public).GetValue(null,null),credentialInterface);
   diagnostic("cnas_credentials_configured",new {source="wisemed_ecard_host",sql_required=false});
  }
  InitializeStep("construct_cnas_context");
  context=Activator.CreateInstance(contextType);
  InitializeStep("construct_provider");
  provider=Activator.CreateInstance(providerMethod.ReturnType);
  Set(provider,"ProviderCode",Value(contract,"cif"));Set(provider,"ProviderCui",Value(contract,"cui"));Set(provider,"ContractNo",Value(contract,"number"));
  Set(provider,"ContractDate",DateTime.ParseExact(Value(contract,"date"),"yyyy-MM-dd",System.Globalization.CultureInfo.InvariantCulture));
  Set(provider,"OrgUnit",Value(contract,"house"));Set(provider,"ProviderType",Value(contract,"provider_type"));
  InitializeStep("attach_session");
  contextType.GetField("LoggingEnabled",Members).SetValue(context,false);
  contextType.GetField("CachingEnabled",Members).SetValue(context,false);
  contextType.GetField("currentSesiuneCard",Members).SetValue(context,session);
  // Invoke CNAS's interceptor itself, including version-specific behavior.
  InitializeStep("construct_cnas_interceptor");
  var interceptor=assembly.GetType("Cnas.Ceas.SmartCard.Novensys.Services.TerminalManagerInterceptor",true);
  Activator.CreateInstance(interceptor,Members,null,new[]{session},null);
  diagnostic("cnas_wrapper_loaded",new {wrapper_version=Version,sdk_version=session.GetType().Assembly.GetName().Version.ToString(),context=contextType.FullName});
  InitializationStep="ready";
 }
 void InitializeStep(string step){InitializationStep=step;diagnostic("cnas_wrapper_initializing",new {step=step,apartment_state=System.Threading.Thread.CurrentThread.GetApartmentState().ToString()});}
 // The CNAS context first resolves ICredentialsProvider from Unity. Without a
 // registration it constructs HttpsCredentialsProvider, which reads activation
 // credentials from the desktop SQL database in its constructor. eCard GetToken
 // does not use this HTTPS provider: it calls the SDK with ProviderData instead.
 // Register a normal, constructed provider at that supported DI boundary. Keep
 // the vendor context constructor and all original card/recovery methods intact.
 static object RegisterStandaloneCredentials(object container,Type credentialInterface){
  if(container==null||!credentialInterface.IsInterface)throw new InvalidOperationException("CNAS credentials dependency is incompatible");
  var baseType=credentialInterface.Assembly.GetType("Cnas.Siui.WebServices.Services.AbstractCredentialsProvider",true);
  var dynamicAssembly=AssemblyBuilder.DefineDynamicAssembly(new AssemblyName("WiseMED.ECardCredentials."+Guid.NewGuid().ToString("N")),AssemblyBuilderAccess.Run);
  var builder=dynamicAssembly.DefineDynamicModule("Credentials").DefineType("WiseMED.StandaloneECardCredentialsProvider",TypeAttributes.Public|TypeAttributes.Sealed,baseType);
  builder.DefineDefaultConstructor(MethodAttributes.Public);
  foreach(var method in baseType.GetMethods(Members).Where(m=>m.IsVirtual&&!m.IsFinal&&(m.IsAbstract||m.Name=="GetCredentials"||m.Name=="GetServerToken"||m.Name=="SetupRequest"))){
   var access=method.IsPublic?MethodAttributes.Public:MethodAttributes.Family;
   var implementation=builder.DefineMethod(method.Name,access|MethodAttributes.Virtual|MethodAttributes.HideBySig,method.ReturnType,method.GetParameters().Select(p=>p.ParameterType).ToArray());
   var il=implementation.GetILGenerator();
   il.Emit(OpCodes.Ldstr,"SIUI HTTPS credentials are not configured in the standalone eCard host; the eCard UM session uses the supplied contract and SDK enrollment.");
   il.Emit(OpCodes.Newobj,typeof(NotSupportedException).GetConstructor(new[]{typeof(string)}));il.Emit(OpCodes.Throw);
   builder.DefineMethodOverride(implementation,method);
  }
  var instance=Activator.CreateInstance(builder.CreateType());
  var unity=container.GetType().GetInterfaces().Single(t=>t.FullName=="Microsoft.Practices.Unity.IUnityContainer");
  var lifetimeType=unity.Assembly.GetType("Microsoft.Practices.Unity.ContainerControlledLifetimeManager",true);
  unity.GetMethod("RegisterInstance").Invoke(container,new object[]{credentialInterface,null,instance,Activator.CreateInstance(lifetimeType)});
  return instance;
 }
 static string Value(Dictionary<string,object> d,string key){object v;return d.TryGetValue(key,out v)?Convert.ToString(v):"";}
 static void Set(object o,string name,object value){o.GetType().GetProperty(name).SetValue(o,value,null);}
 public object Call(string name,params object[] args){
  diagnostic("cnas_wrapper_call_started",new {method=name,wrapper_version=Version});
  try{
   for(var t=context.GetType();t!=null;t=t.BaseType){var methods=t.GetMethods(Members|BindingFlags.DeclaredOnly).Where(m=>m.Name==name&&m.GetParameters().Length==args.Length).ToArray();if(methods.Length==1){var result=methods[0].Invoke(context,args);diagnostic("cnas_wrapper_call_finished",new {method=name,wrapper_version=Version});return result;}}
   throw new MissingMethodException(contextType.FullName,name);
  }catch(Exception ex){diagnostic("cnas_wrapper_call_failed",new {method=name,wrapper_version=Version,exception_type=ex.GetBaseException().GetType().FullName});throw;}
 }
 public string AcquireToken(){
  var token=Convert.ToString(Call("GetToken",credentials,provider,null,null));
  contextType.GetProperty("Token",Members).GetSetMethod(true).Invoke(context,new object[]{token});
  return token;
 }
 // The desktop ReadData/WriteData methods additionally translate diagnoses
 // through the CNAS SQL database. Use its actual operation evaluator around
 // the SDK call so token refresh and FixKnownCardIssues still run in the vendor
 // DLL, while retaining the raw SDK data and per-field result codes.
 public int Evaluate(Func<int> operation){
  var result=Call("EvaluateOperationResult",operation,provider);
  return Convert.ToInt32(Call("GetOperationResultCode",result));
 }
 public string Token {get{return Convert.ToString(contextType.GetProperty("Token",Members).GetValue(context,null));}}
 public byte[] Sign(byte[] bytes){return (byte[])Call("ComputeHash",bytes);}
 public void Execute(string operation){
  string method=operation=="activate"?"Activate":operation=="change-pin"?"ChangePin":operation=="reset-pin"?"ResetPin":null;
  var result=Call(method,provider);
  if(!(result is bool)||(bool)result==false)throw new InvalidOperationException("CNAS wrapper did not confirm operation success");
 }
 public int? SDKError(Exception exception){
  for(var ex=exception;ex!=null;ex=ex.InnerException){
   foreach(var name in new[]{"OperationResult","Result"}){var p=ex.GetType().GetProperty(name);if(p==null||!p.PropertyType.IsEnum)continue;try{return Convert.ToInt32(Call("GetOperationResultCode",p.GetValue(ex,null)));}catch{}}
  }
  return null;
 }
}
