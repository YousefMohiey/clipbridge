using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Drawing.Imaging;

namespace ClipBridge;

public sealed class RemoteState
{
    public int rev { get; set; }
    public string? type { get; set; }
    public string? hash { get; set; }
    public string? text { get; set; }
    public int size { get; set; }
}

public sealed class ClipItem
{
    public DateTime When { get; set; } = DateTime.Now;
    public string Kind { get; set; } = "text";
    public string Snippet { get; set; } = "";
    public string? Text { get; set; }
    public byte[]? Png { get; set; }
    public string Origin { get; set; } = "local";
}

public sealed class Settings
{
    public string Server { get; set; } = "http://192.168.1.8:1717";
    public bool SyncText { get; set; } = true;
    public bool SyncImages { get; set; } = true;
    public bool StartHidden { get; set; }
}

public static class Paths
{
    public static readonly string Dir = Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "ClipBridge");
    public static string SettingsFile => Path.Combine(Dir, "settings.json");
    public static string LogFile => Path.Combine(Dir, "app.log");

    public static void Ensure()
    {
        Directory.CreateDirectory(Dir);
    }

    public static void Log(string line)
    {
        try
        {
            Ensure();
            File.AppendAllText(LogFile, $"{DateTime.Now:yyyy-MM-dd HH:mm:ss}  {line}{Environment.NewLine}");
        }
        catch { }
    }

    public static Settings Load()
    {
        try
        {
            if (File.Exists(SettingsFile))
                return JsonSerializer.Deserialize<Settings>(File.ReadAllText(SettingsFile)) ?? new Settings();
        }
        catch { }
        return new Settings();
    }

    public static void Save(Settings s)
    {
        try
        {
            Ensure();
            File.WriteAllText(SettingsFile, JsonSerializer.Serialize(s, new JsonSerializerOptions { WriteIndented = true }));
        }
        catch { }
    }
}

public sealed class BridgeEngine
{
    private readonly HttpClient _http = new() { Timeout = TimeSpan.FromSeconds(12) };
    private string _lastHash = "";
    private int _lastRev = -1;
    private bool _busy;

    public string Server { get; set; } = "http://192.168.1.8:1717";
    public bool Paused { get; set; }
    public bool SyncText { get; set; } = true;
    public bool SyncImages { get; set; } = true;

    public event Action<bool, string>? StatusChanged;
    public event Action<ClipItem>? ClipSeen;

    private static string Trim(string s)
    {
        var one = s.Replace("\r", " ").Replace("\n", " ").Trim();
        return one.Length <= 80 ? one : one[..77] + "...";
    }

    private static string Hash(string kind, byte[]? png, string? text)
    {
        using var sha = SHA256.Create();
        var head = Encoding.UTF8.GetBytes(kind);
        var body = png ?? Encoding.UTF8.GetBytes(text ?? "");
        var buf = new byte[head.Length + body.Length];
        Buffer.BlockCopy(head, 0, buf, 0, head.Length);
        Buffer.BlockCopy(body, 0, buf, head.Length, body.Length);
        return Convert.ToHexString(sha.ComputeHash(buf));
    }

    private ClipItem? ReadLocal()
    {
        try
        {
            if (SyncImages && Clipboard.ContainsImage())
            {
                using var img = Clipboard.GetImage();
                if (img != null)
                {
                    using var ms = new MemoryStream();
                    img.Save(ms, ImageFormat.Png);
                    var bytes = ms.ToArray();
                    return new ClipItem { Kind = "image", Png = bytes, Snippet = $"Image {bytes.Length / 1024} KB", Origin = "this PC" };
                }
            }
            if (SyncText && Clipboard.ContainsText())
            {
                var t = Clipboard.GetText();
                if (!string.IsNullOrEmpty(t))
                    return new ClipItem { Kind = "text", Text = t, Snippet = Trim(t), Origin = "this PC" };
            }
        }
        catch { }
        return null;
    }

    private async Task<bool> PushAsync(ClipItem item)
    {
        try
        {
            HttpResponseMessage resp;
            if (item.Kind == "image" && item.Png != null)
            {
                using var content = new ByteArrayContent(item.Png);
                content.Headers.ContentType = new System.Net.Http.Headers.MediaTypeHeaderValue("image/png");
                resp = await _http.PostAsync(Server + "/set", content);
            }
            else
            {
                var json = JsonSerializer.Serialize(new { text = item.Text ?? "" });
                using var content = new StringContent(json, Encoding.UTF8, "application/json");
                resp = await _http.PostAsync(Server + "/set", content);
            }
            return resp.IsSuccessStatusCode;
        }
        catch (Exception ex)
        {
            Paths.Log("push failed: " + ex.Message);
            return false;
        }
    }

    private async Task<RemoteState?> GetStateAsync()
    {
        var json = await _http.GetStringAsync(Server + "/state");
        return JsonSerializer.Deserialize<RemoteState>(json, new JsonSerializerOptions { PropertyNameCaseInsensitive = true });
    }

    private async Task<byte[]?> GetImageAsync()
    {
        return await _http.GetByteArrayAsync(Server + "/image");
    }

    private void ApplyText(string text)
    {
        try { Clipboard.SetText(text); } catch (Exception ex) { Paths.Log("apply text failed: " + ex.Message); }
    }

    private void ApplyImage(byte[] png)
    {
        try
        {
            using var ms = new MemoryStream(png);
            using var src = Image.FromStream(ms);
            using var copy = new Bitmap(src);
            Clipboard.SetImage(copy);
        }
        catch (Exception ex) { Paths.Log("apply image failed: " + ex.Message); }
    }

    public async Task PollAsync()
    {
        if (_busy || Paused) return;
        _busy = true;
        try
        {
            var local = ReadLocal();
            if (local != null)
            {
                var h = Hash(local.Kind, local.Png, local.Text);
                if (h != _lastHash)
                {
                    if (await PushAsync(local))
                    {
                        _lastHash = h;
                        ClipSeen?.Invoke(local);
                    }
                }
            }

            var st = await GetStateAsync();
            if (st?.type != null)
            {
                if (st.rev != _lastRev && st.hash != _lastHash)
                {
                    if (st.type == "image")
                    {
                        var png = await GetImageAsync();
                        if (png is { Length: > 0 })
                        {
                            ApplyImage(png);
                            ClipSeen?.Invoke(new ClipItem
                            {
                                Kind = "image", Png = png, Origin = "host",
                                Snippet = $"Image {png.Length / 1024} KB"
                            });
                        }
                    }
                    else
                    {
                        ApplyText(st.text ?? "");
                        ClipSeen?.Invoke(new ClipItem
                        {
                            Kind = "text", Text = st.text, Origin = "host", Snippet = Trim(st.text ?? "")
                        });
                    }
                    _lastHash = st.hash ?? "";
                }
                _lastRev = st.rev;
            }
            StatusChanged?.Invoke(true, "Connected");
        }
        catch (Exception ex)
        {
            StatusChanged?.Invoke(false, "Not connected");
            Paths.Log("poll failed: " + ex.Message);
        }
        finally
        {
            _busy = false;
        }
    }

    public async Task<bool> SendCurrentAsync()
    {
        var local = ReadLocal();
        if (local == null) return false;
        if (await PushAsync(local))
        {
            _lastHash = Hash(local.Kind, local.Png, local.Text);
            ClipSeen?.Invoke(local);
            return true;
        }
        return false;
    }
}
