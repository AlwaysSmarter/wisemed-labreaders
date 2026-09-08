using System.Reflection;
using System.Runtime.CompilerServices;
using System.Text.Json;

// Use the actual supplied DLL without constructing its Windows Forms control or
// opening a tablet. Initialize only the signature's storage and lock, leaving
// encryption/compression and optional metadata at WiseMED's zero defaults.
if (args.Length != 2) throw new ArgumentException("Pass SigPlusNET.dll and topaz-sigstring.json paths");
var asm = Assembly.LoadFrom(Path.GetFullPath(args[0]));
var type = asm.GetType("Topaz.Signature", true)!;
var obj = RuntimeHelpers.GetUninitializedObject(type);
const BindingFlags flags = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
type.GetField("PenStateLock", flags)!.SetValue(obj, new object());
type.GetField("StrokeList", flags)!.SetValue(obj, Activator.CreateInstance(asm.GetType("Topaz.SigStrokeList")!, true));
object? Call(string name, params object[] values) => type.GetMethod(name, flags)!.Invoke(obj, values);
using var fixture = JsonDocument.Parse(File.ReadAllText(args[1]));
var first = true;
foreach (var point in fixture.RootElement.GetProperty("points").EnumerateArray()) {
    if (first || point[2].GetInt32() == 0) Call("NewStroke");
    first = false;
    Call("AddPointToStroke", point[0].GetInt32(), point[1].GetInt32(), 0, point[2].GetInt32());
}
var expected = fixture.RootElement.GetProperty("sigenc").GetString();
if ((string?)Call("GetSigString") != expected) throw new Exception("Original DLL export differs from Go fixture");
if (!(bool)Call("SetSigString", expected!)!) throw new Exception("Original DLL rejected Go fixture");
if ((string?)Call("GetSigString") != expected) throw new Exception("Original DLL round trip changed data");
Console.WriteLine("PASS: original SigPlusNET GetSigString and SetSigString match the Go fixture.");
