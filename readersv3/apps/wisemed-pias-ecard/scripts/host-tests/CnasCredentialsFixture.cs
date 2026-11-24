using System;
using System.Collections.Generic;
using System.Net;
using System.Security.Cryptography.X509Certificates;

namespace Cnas.Siui.WebServices.Interfaces {
 public interface ICredentialsProvider {
  string Username{get;set;}string Password{get;set;}string HostAddress{get;set;}int CommandType{get;set;}X509Certificate2 Certificate{get;set;}
  ICredentials GetCredentials(Uri uri,bool digest=true);string GetServerToken(Uri uri=null);bool IsSecured(Type type);Uri GetServiceUri(Type type);void SetServiceUri(Type type,Uri uri);void ClearTokensAndCookies();void SetupRequest(WebRequest request);
 }
}
namespace Cnas.Siui.WebServices.Services {
 public abstract class AbstractCredentialsProvider:Cnas.Siui.WebServices.Interfaces.ICredentialsProvider {
  public virtual string Username{get;set;}public virtual string Password{get;set;}public virtual string HostAddress{get;set;}public virtual int CommandType{get;set;}public virtual X509Certificate2 Certificate{get;set;}
  public virtual ICredentials GetCredentials(Uri uri,bool digest=true){return null;}public virtual string GetServerToken(Uri uri=null){return null;}
  public virtual bool IsSecured(Type type){return GetServiceUri(type).Scheme=="https";}
  public abstract Uri GetServiceUri(Type type);public abstract void SetServiceUri(Type type,Uri uri);
  public virtual void ClearTokensAndCookies(){}public virtual void SetupRequest(WebRequest request){}
 }
}
namespace Microsoft.Practices.Unity {
 public abstract class LifetimeManager{}
 public sealed class ContainerControlledLifetimeManager:LifetimeManager{}
 public interface IUnityContainer {IUnityContainer RegisterInstance(Type type,string name,object instance,LifetimeManager lifetime);}
 public sealed class UnityContainer:IUnityContainer {
  readonly Dictionary<Type,object> registrations=new Dictionary<Type,object>();
  public IUnityContainer RegisterInstance(Type type,string name,object instance,LifetimeManager lifetime){if(!type.IsInstanceOfType(instance)||lifetime==null)throw new Exception("Invalid credentials registration");registrations[type]=instance;return this;}
  public object Resolve(Type type){object value;return registrations.TryGetValue(type,out value)?value:null;}
 }
}
public static class CnasCredentialsFixture {
 public static Microsoft.Practices.Unity.UnityContainer Container=new Microsoft.Practices.Unity.UnityContainer();
 public static int SQLFallbacks;
 public static Cnas.Siui.WebServices.Interfaces.ICredentialsProvider Resolve(){
  var provider=(Cnas.Siui.WebServices.Interfaces.ICredentialsProvider)Container.Resolve(typeof(Cnas.Siui.WebServices.Interfaces.ICredentialsProvider));
  if(provider==null){SQLFallbacks++;throw new InvalidOperationException("HttpsCredentialsProvider fallback would query CNAS activation SQL");}
  return provider;
 }
}
