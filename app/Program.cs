using System.Diagnostics;

namespace ClipBridge;

internal static class Program
{
    [STAThread]
    private static void Main()
    {
        using var mutex = new Mutex(true, "ClipBridge.SingleInstance", out var first);
        if (!first)
        {
            MessageBox.Show("ClipBridge is already running. Look for it in the tray.", "ClipBridge");
            return;
        }

        ApplicationConfiguration.Initialize();
        Paths.Ensure();
        Paths.Log("start");

        var settings = Paths.Load();
        var engine = new BridgeEngine
        {
            Server = settings.Server,
            SyncText = settings.SyncText,
            SyncImages = settings.SyncImages
        };

        Application.ThreadException += (_, e) => Paths.Log("thread exception: " + e.Exception);
        AppDomain.CurrentDomain.UnhandledException += (_, e) => Paths.Log("unhandled: " + e.ExceptionObject);

        Application.Run(new MainForm(engine, settings));
    }
}
