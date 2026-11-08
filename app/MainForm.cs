using System.Drawing.Imaging;

namespace ClipBridge;

public sealed class MainForm : Form
{
    private static readonly Color Bg = Color.FromArgb(14, 17, 22);
    private static readonly Color Panel = Color.FromArgb(21, 26, 33);
    private static readonly Color Ink = Color.FromArgb(230, 237, 243);
    private static readonly Color Dim = Color.FromArgb(139, 152, 168);
    private static readonly Color Accent = Color.FromArgb(59, 130, 246);
    private static readonly Color Good = Color.FromArgb(34, 197, 94);
    private static readonly Color Bad = Color.FromArgb(239, 68, 68);

    private readonly BridgeEngine _engine;
    private readonly Settings _settings;
    private readonly System.Windows.Forms.Timer _timer = new() { Interval = 700 };
    private readonly ListView _list = new();
    private readonly Label _status = new();
    private readonly TextBox _server = new();
    private readonly CheckBox _text = new();
    private readonly CheckBox _images = new();
    private readonly CheckBox _pause = new();
    private readonly CheckBox _start = new();
    private readonly NotifyIcon _tray = new();
    private readonly List<ClipItem> _items = new();

    public MainForm(BridgeEngine engine, Settings settings)
    {
        _engine = engine;
        _settings = settings;

        Text = "ClipBridge";
        Size = new Size(520, 620);
        MinimumSize = new Size(460, 500);
        StartPosition = FormStartPosition.CenterScreen;
        BackColor = Bg;
        ForeColor = Ink;
        Font = new Font("Segoe UI", 9F);

        BuildUi();
        BuildTray();

        _engine.StatusChanged += OnStatus;
        _engine.ClipSeen += OnClip;

        _timer.Tick += async (_, _) => await _engine.PollAsync();
        _timer.Start();

        FormClosing += (_, e) =>
        {
            if (e.CloseReason == CloseReason.UserClosing)
            {
                e.Cancel = true;
                Hide();
                _tray.ShowBalloonTip(1500, "ClipBridge", "Still running here. Double click the tray icon to open.", ToolTipIcon.Info);
            }
        };
    }

    private void BuildUi()
    {
        var root = new TableLayoutPanel
        {
            Dock = DockStyle.Fill,
            ColumnCount = 1,
            RowCount = 5,
            Padding = new Padding(18, 14, 18, 12),
            BackColor = Bg
        };
        root.RowStyles.Add(new RowStyle(SizeType.Absolute, 54));
        root.RowStyles.Add(new RowStyle(SizeType.Absolute, 42));
        root.RowStyles.Add(new RowStyle(SizeType.Absolute, 34));
        root.RowStyles.Add(new RowStyle(SizeType.Percent, 100));
        root.RowStyles.Add(new RowStyle(SizeType.Absolute, 52));

        var head = new Panel { Dock = DockStyle.Fill, BackColor = Bg };
        var title = new Label
        {
            Text = "ClipBridge",
            Font = new Font("Segoe UI Semibold", 16F),
            ForeColor = Ink,
            AutoSize = true,
            Location = new Point(0, 2)
        };
        _status.Text = "Starting";
        _status.ForeColor = Dim;
        _status.AutoSize = true;
        _status.Location = new Point(2, 32);
        head.Controls.Add(title);
        head.Controls.Add(_status);
        root.Controls.Add(head, 0, 0);

        var serverRow = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 2, BackColor = Bg };
        serverRow.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
        serverRow.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 78));
        _server.Text = _settings.Server;
        _server.Dock = DockStyle.Fill;
        _server.BackColor = Panel;
        _server.ForeColor = Ink;
        _server.BorderStyle = BorderStyle.FixedSingle;
        _server.Margin = new Padding(0, 4, 8, 6);
        var apply = new Button
        {
            Text = "Apply",
            Dock = DockStyle.Fill,
            FlatStyle = FlatStyle.Flat,
            BackColor = Panel,
            ForeColor = Ink,
            Margin = new Padding(0, 4, 0, 6)
        };
        apply.FlatAppearance.BorderColor = Color.FromArgb(48, 58, 71);
        apply.Click += (_, _) =>
        {
            _engine.Server = _server.Text.Trim().TrimEnd('/');
            _settings.Server = _engine.Server;
            Paths.Save(_settings);
            Paths.Log("server set to " + _engine.Server);
        };
        serverRow.Controls.Add(_server, 0, 0);
        serverRow.Controls.Add(apply, 1, 0);
        root.Controls.Add(serverRow, 0, 1);

        var opts = new FlowLayoutPanel { Dock = DockStyle.Fill, BackColor = Bg, WrapContents = false };
        _text.Text = "Text";
        _images.Text = "Images";
        _pause.Text = "Pause";
        foreach (var cb in new[] { _text, _images, _pause })
        {
            cb.ForeColor = Ink;
            cb.AutoSize = true;
            cb.Margin = new Padding(0, 6, 20, 0);
        }
        _start.Text = "Start with Windows";
        _start.ForeColor = Ink;
        _start.AutoSize = true;
        _start.Margin = new Padding(0, 6, 20, 0);
        _start.Checked = StartupEnabled();
        _start.CheckedChanged += (_, _) =>
        {
            SetStartup(_start.Checked);
            Paths.Log("start with windows: " + _start.Checked);
        };
        _text.Checked = _settings.SyncText;
        _images.Checked = _settings.SyncImages;
        _text.CheckedChanged += (_, _) => { _engine.SyncText = _text.Checked; _settings.SyncText = _text.Checked; Paths.Save(_settings); };
        _images.CheckedChanged += (_, _) => { _engine.SyncImages = _images.Checked; _settings.SyncImages = _images.Checked; Paths.Save(_settings); };
        _pause.CheckedChanged += (_, _) => { _engine.Paused = _pause.Checked; };
        opts.Controls.Add(_text);
        opts.Controls.Add(_images);
        opts.Controls.Add(_pause);
        opts.Controls.Add(_start);
        root.Controls.Add(opts, 0, 2);

        _list.Dock = DockStyle.Fill;
        _list.View = View.Details;
        _list.FullRowSelect = true;
        _list.MultiSelect = false;
        _list.HideSelection = false;
        _list.BackColor = Panel;
        _list.ForeColor = Ink;
        _list.BorderStyle = BorderStyle.None;
        _list.Columns.Add("Time", 72);
        _list.Columns.Add("From", 70);
        _list.Columns.Add("Type", 56);
        _list.Columns.Add("Preview", 250);
        _list.DoubleClick += (_, _) => CopySelected();
        root.Controls.Add(_list, 0, 3);

        var buttons = new FlowLayoutPanel { Dock = DockStyle.Fill, BackColor = Bg, WrapContents = false };
        buttons.Controls.Add(MakeButton("Copy", CopySelected));
        buttons.Controls.Add(MakeButton("Send now", async () =>
        {
            var ok = await _engine.SendCurrentAsync();
            SetStatus(ok, ok ? "Sent to host" : "Nothing to send");
        }));
        buttons.Controls.Add(MakeButton("Open log", () =>
        {
            Paths.Ensure();
            if (File.Exists(Paths.LogFile)) System.Diagnostics.Process.Start("notepad.exe", Paths.LogFile);
            else MessageBox.Show("No log yet.", "ClipBridge");
        }));
        root.Controls.Add(buttons, 0, 4);

        Controls.Add(root);
    }

    private Button MakeButton(string text, Action action)
    {
        var b = new Button
        {
            Text = text,
            AutoSize = false,
            Size = new Size(104, 34),
            FlatStyle = FlatStyle.Flat,
            BackColor = Panel,
            ForeColor = Ink,
            Margin = new Padding(0, 6, 10, 0)
        };
        b.FlatAppearance.BorderColor = Color.FromArgb(48, 58, 71);
        b.Click += (_, _) => action();
        return b;
    }

    private Button MakeButton(string text, Func<Task> action)
    {
        return MakeButton(text, () => _ = action());
    }

    private void BuildTray()
    {
        var menu = new ContextMenuStrip();
        menu.Items.Add("Open", null, (_, _) => { Show(); WindowState = FormWindowState.Normal; Activate(); });
        menu.Items.Add("Send clipboard now", null, async (_, _) => await _engine.SendCurrentAsync());
        menu.Items.Add(new ToolStripSeparator());
        menu.Items.Add("Exit", null, (_, _) =>
        {
            _timer.Stop();
            _tray.Visible = false;
            Application.Exit();
        });

        _tray.Icon = SystemIcons.Application;
        _tray.Text = "ClipBridge";
        _tray.Visible = true;
        _tray.ContextMenuStrip = menu;
        _tray.DoubleClick += (_, _) => { Show(); WindowState = FormWindowState.Normal; Activate(); };
    }

    private void OnStatus(bool ok, string message)
    {
        if (InvokeRequired) { BeginInvoke(() => OnStatus(ok, message)); return; }
        SetStatus(ok, message);
    }

    private void SetStatus(bool ok, string message)
    {
        _status.Text = (ok ? "\u25CF " : "\u25CB ") + message;
        _status.ForeColor = ok ? Good : Bad;
        _tray.Text = "ClipBridge - " + message;
    }

    private void OnClip(ClipItem item)
    {
        if (InvokeRequired) { BeginInvoke(() => OnClip(item)); return; }
        _items.Insert(0, item);
        if (_items.Count > 60) _items.RemoveAt(_items.Count - 1);

        var row = new ListViewItem(item.When.ToString("HH:mm:ss"));
        row.SubItems.Add(item.Origin);
        row.SubItems.Add(item.Kind);
        row.SubItems.Add(item.Snippet);
        row.Tag = item;
        _list.Items.Insert(0, row);
        if (_list.Items.Count > 60) _list.Items.RemoveAt(_list.Items.Count - 1);
        Paths.Log($"{item.Origin} {item.Kind}: {item.Snippet}");
    }

    private void CopySelected()
    {
        if (_list.SelectedItems.Count == 0) return;
        if (_list.SelectedItems[0].Tag is not ClipItem item) return;
        try
        {
            if (item.Kind == "image" && item.Png != null)
            {
                using var ms = new MemoryStream(item.Png);
                using var src = Image.FromStream(ms);
                using var copy = new Bitmap(src);
                Clipboard.SetImage(copy);
            }
            else if (item.Text != null)
            {
                Clipboard.SetText(item.Text);
            }
            SetStatus(true, "Copied");
        }
        catch (Exception ex)
        {
            Paths.Log("copy failed: " + ex.Message);
        }
    }

    private static bool StartupEnabled()
    {
        try
        {
            using var key = Microsoft.Win32.Registry.CurrentUser.OpenSubKey(
                @"SOFTWARE\Microsoft\Windows\CurrentVersion\Run", false);
            return key?.GetValue("ClipBridge") != null;
        }
        catch { return false; }
    }

    private static void SetStartup(bool on)
    {
        try
        {
            using var key = Microsoft.Win32.Registry.CurrentUser.OpenSubKey(
                @"SOFTWARE\Microsoft\Windows\CurrentVersion\Run", true);
            if (key == null) return;
            if (on) key.SetValue("ClipBridge", "\"" + Application.ExecutablePath + "\"");
            else key.DeleteValue("ClipBridge", false);
        }
        catch (Exception ex) { Paths.Log("startup toggle failed: " + ex.Message); }
    }

    protected override void Dispose(bool disposing)
    {
        if (disposing)
        {
            _timer.Dispose();
            _tray.Dispose();
        }
        base.Dispose(disposing);
    }
}
