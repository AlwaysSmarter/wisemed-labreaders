// Test assembly fixture verifies that the adapter delegates into the loaded DLL.
// This is not a replacement for Windows tests against the vendor wrapper.
using System;
using System.Collections.Generic;
namespace Cnas.Ceas.SmartCard.Novensys.Services {
 public sealed class ProviderData {public string ProviderCode{get;set;}public string ProviderCui{get;set;}public string ContractNo{get;set;}public DateTime ContractDate{get;set;}public string OrgUnit{get;set;}public string ProviderType{get;set;}}
 public enum OperationResult {OK=0,BadToken=-2,UM=-29}
 public class NovensysSmartCardContext {
  public static bool FailConstruction;
  public static bool RequireRegisteredCredentials;
  public static bool FailWithUnthrownInner;
  public NovensysSmartCardContext(){if(FailWithUnthrownInner)throw new TypeInitializationException("Fixture.CnasShell",new System.ComponentModel.Win32Exception(233,"fixture broken pipe"));if(FailConstruction)throw new System.ComponentModel.Win32Exception(5,"fixture Windows initialization error");}
  protected bool LoggingEnabled=true,CachingEnabled=true;
  private readonly Cnas.Siui.WebServices.Interfaces.ICredentialsProvider credentialsProvider=RequireRegisteredCredentials?CnasCredentialsFixture.Resolve():null;
  private object currentSesiuneCard;
  protected string Token{get;private set;}
  public static int Evaluations,Refreshes,Signs,Activations;
  public virtual ProviderData GetProviderData(){throw new Exception("Desktop provider lookup must not be used");}
  protected string GetToken(Cnas.Siui.WebServices.Interfaces.ICredentialsProvider credentials,ProviderData provider,Uri service,Uri ocsp){if(provider.ContractNo!="test")throw new Exception("wrong provider");var session=(FakeSession)currentSesiuneCard;Token=session.Offline?new string('0',32):"fixture-token";session.Token=Token;return Token;}
  protected OperationResult EvaluateOperationResult(Func<int> operation,ProviderData provider=null){Evaluations++;int result=operation();if(result==-2){Refreshes++;Token=GetToken(null,provider,null,null);result=operation();}return (OperationResult)result;}
  protected int GetOperationResultCode(OperationResult result){return (int)result;}
  protected byte[] ComputeHash(byte[] bytes){Signs++;return bytes;}
  public bool Activate(ProviderData provider){Activations++;return provider.ContractNo=="test";}
  public bool ChangePin(ProviderData provider){return true;}public bool ResetPin(ProviderData provider){return true;}
 }
 internal sealed class TerminalManagerInterceptor { public static int Calls;public TerminalManagerInterceptor(object session){Calls++;} }
}
