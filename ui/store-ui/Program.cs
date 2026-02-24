using System.Diagnostics;
using System.Globalization;
using System.Text.Encodings.Web;
using System.Text.Json;
using System.Text.Json.Serialization;
using Microsoft.Web.WebView2.Core;
using Microsoft.Web.WebView2.WinForms;

namespace AppCenter.StoreUI;

internal static class Program
{
    [STAThread]
    private static void Main()
    {
        ApplicationConfiguration.Initialize();
        Application.Run(new StoreForm());
    }
}

internal sealed class StoreForm : Form
{
    private readonly WebView2 _web = new() { Dock = DockStyle.Fill };
    private readonly StoreBackend _backend = new();
    private bool _ready;

    public StoreForm()
    {
        Text = "AppCenter Store";
        StartPosition = FormStartPosition.CenterScreen;
        Width = 1480;
        Height = 1040;
        MinimumSize = new Size(1200, 900);
        Controls.Add(_web);
        Shown += async (_, _) => await InitAsync();
    }

    private async Task InitAsync()
    {
        try
        {
            var userDataDir = Path.Combine(
                Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
                "AppCenter",
                "StoreUI",
                "WebView2Data");
            Directory.CreateDirectory(userDataDir);

            var env = await CoreWebView2Environment.CreateAsync(userDataFolder: userDataDir);
            await _web.EnsureCoreWebView2Async(env);
            _web.CoreWebView2.Settings.AreDefaultContextMenusEnabled = false;
            _web.CoreWebView2.Settings.AreBrowserAcceleratorKeysEnabled = true;
            _web.CoreWebView2.WebMessageReceived += OnWebMessage;
            _web.NavigateToString(StoreHtml);
            _ready = true;
            await PushStoreAsync();
        }
        catch (Exception ex)
        {
            StoreLog.Write("InitAsync failed: " + ex);
            MessageBox.Show(this, "Modern store arayüzü başlatılamadı:\n\n" + ex.Message, "AppCenter Store", MessageBoxButtons.OK, MessageBoxIcon.Error);
            Close();
        }
    }

    private async void OnWebMessage(object? sender, CoreWebView2WebMessageReceivedEventArgs e)
    {
        try
        {
            var msg = JsonSerializer.Deserialize<WebMessage>(e.WebMessageAsJson, JsonOpts()) ?? new WebMessage();
            switch ((msg.Type ?? "").Trim().ToLowerInvariant())
            {
                case "refresh":
                    await PushStoreAsync();
                    break;
                case "install":
                    if (msg.AppId <= 0)
                    {
                        await EmitToastAsync("Geçersiz uygulama seçimi.");
                        return;
                    }
                    var ack = _backend.RequestInstall(msg.AppId);
                    await _web.ExecuteScriptAsync(
                        $"window.applyInstallAck({msg.AppId}, '{JsEscape(ack.QueueStatus)}');");
                    await EmitToastAsync(ack.Message);
                    await PushStoreAsync();
                    break;
            }
        }
        catch (Exception ex)
        {
            StoreLog.Write("OnWebMessage failed: " + ex);
            await EmitToastAsync("Hata: " + JsEscape(ex.Message));
        }
    }

    private async Task PushStoreAsync()
    {
        if (!_ready)
        {
            return;
        }

        try
        {
            var apps = _backend.FetchStoreApps();
            var json = JsonSerializer.Serialize(apps, JsonOpts());
            await _web.ExecuteScriptAsync($"window.renderStore({json});");
        }
        catch (Exception ex)
        {
            StoreLog.Write("PushStoreAsync failed: " + ex);
            await EmitToastAsync("Store yüklenemedi: " + JsEscape(ex.Message));
        }
    }

    private Task EmitToastAsync(string message) =>
        _web.ExecuteScriptAsync($"window.showToast('{JsEscape(message)}');");

    private static string JsEscape(string s) =>
        (s ?? "").Replace("\\", "\\\\").Replace("'", "\\'").Replace("\r", " ").Replace("\n", " ");

    private static JsonSerializerOptions JsonOpts() => new()
    {
        PropertyNameCaseInsensitive = true,
        Encoder = JavaScriptEncoder.UnsafeRelaxedJsonEscaping,
    };

    private const string StoreHtml = """
<!doctype html>
<html lang="tr">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>AppCenter Store</title>
  <style>
    :root{
      --bg:#f2f5f8;
      --ink:#1f2937;
      --muted:#6b7280;
      --card:#ffffff;
      --line:#e5e7eb;
      --brand:#0f62fe;
      --ok:#138a36;
    }
    *{box-sizing:border-box}
    body{margin:0;background:radial-gradient(1200px 500px at 10% -10%, #dbeafe 0%, var(--bg) 45%), var(--bg); font-family:'Segoe UI Variable Text','Segoe UI',Arial,sans-serif;color:var(--ink)}
    .shell{max-width:1100px;margin:0 auto;padding:22px}
    .top{display:flex;align-items:center;justify-content:space-between;gap:14px;margin-bottom:16px;flex-wrap:wrap}
    h1{margin:0;font-size:24px;font-weight:700;letter-spacing:.2px}
    .actions{display:flex;gap:10px;align-items:center;flex-wrap:wrap;justify-content:flex-end;flex:1;min-width:520px}
    .hint{font-size:12px;color:var(--muted)}
    .counter{font-size:12px;color:#475569;background:#e2e8f0;padding:3px 8px;border-radius:999px}
    input{width:420px;max-width:100%;padding:11px 12px;border:1px solid #d1d5db;border-radius:12px;font-size:14px;outline:none;background:#fff;flex:1;min-width:260px}
    input:focus{border-color:#93c5fd;box-shadow:0 0 0 3px rgba(59,130,246,.15)}
    button{border:0;border-radius:12px;padding:10px 14px;background:var(--brand);color:#fff;font-weight:600;cursor:pointer}
    button.secondary{background:#334155}
    button.ghost{background:#e2e8f0;color:#0f172a}
    button:disabled{opacity:.6;cursor:not-allowed}
    .grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(300px,1fr));gap:12px}
    .card{background:var(--card);border:1px solid var(--line);border-radius:16px;padding:14px;box-shadow:0 4px 18px rgba(15,23,42,.04)}
    .row{display:flex;align-items:center;justify-content:space-between;gap:10px}
    .left{display:flex;align-items:center;gap:10px;min-width:0}
    .icon{width:63px;height:63px;border-radius:12px;background:#f1f5f9;border:1px solid #e2e8f0;object-fit:cover;flex:0 0 63px}
    .icon-fallback{display:flex;align-items:center;justify-content:center;font-size:12px;color:#64748b;font-weight:700}
    .name{font-size:16px;font-weight:700}
    .meta{font-size:12px;color:var(--muted);margin-top:4px}
    .desc{font-size:13px;color:#4b5563;margin-top:10px;line-height:1.4;min-height:36px}
    .warn{font-size:12px;color:#92400e;margin-top:8px;line-height:1.35;background:#fffbeb;border:1px solid #fde68a;border-radius:10px;padding:8px}
    .err{font-size:12px;color:#b91c1c;margin-top:8px;line-height:1.35;background:#fef2f2;border:1px solid #fecaca;border-radius:10px;padding:8px}
    .warn details,.err details{margin-top:4px}
    .warn summary,.err summary{cursor:pointer;font-weight:600}
    .pill{display:inline-block;padding:3px 8px;border-radius:999px;font-size:11px;background:#eef2ff;color:#3730a3;margin-right:6px}
    .pill.pending{background:#fff7ed;color:#9a3412}
    .pill.downloading{background:#eff6ff;color:#1d4ed8}
    .pill.installing{background:#f0fdf4;color:#166534}
    .pill.failed{background:#fef2f2;color:#b91c1c}
    .installed-btn{border:0;border-radius:12px;padding:10px 14px;background:var(--ok);color:#fff;font-weight:700;cursor:not-allowed;opacity:.95}
    .empty{padding:18px;text-align:center;color:#64748b}
    .spin{display:inline-block;width:10px;height:10px;border-radius:999px;border:2px solid #94a3b8;border-top-color:transparent;animation:sp 1s linear infinite;margin-right:6px;vertical-align:-1px}
    @keyframes sp{to{transform:rotate(360deg)}}
    .toast{position:fixed;right:16px;bottom:16px;background:#0f172a;color:#fff;padding:10px 14px;border-radius:10px;font-size:13px;display:none}
    @media (max-width:1200px){
      .actions{min-width:100%;justify-content:flex-start}
      .hint{order:1}
      .counter{order:2}
      input{order:3;flex-basis:100%}
      #clear{order:4}
      #refresh{order:5}
    }
  </style>
</head>
<body>
  <div class="shell">
    <div class="top">
      <h1>AppCenter Store</h1>
      <div class="actions">
        <span id="sync" class="hint">Canlı takip: aktif</span>
        <span id="count" class="counter">0 uygulama</span>
        <input id="q" placeholder="Uygulama ara..." />
        <button class="ghost" id="clear">Temizle</button>
        <button class="secondary" id="refresh">Yenile</button>
      </div>
    </div>
    <div id="cards" class="grid"></div>
  </div>
  <div id="toast" class="toast"></div>
  <script>
    let apps = [];
    let lastRefreshAt = null;
    const transientState = {};
    const installingIds = {};
    const cards = document.getElementById('cards');
    const q = document.getElementById('q');
    const sync = document.getElementById('sync');
    const count = document.getElementById('count');
    const refreshBtn = document.getElementById('refresh');
    const clearBtn = document.getElementById('clear');
    refreshBtn.addEventListener('click', ()=> requestRefresh(true));
    clearBtn.addEventListener('click', ()=>{ q.value=''; render(); q.focus(); });
    q.addEventListener('input', render);

    function post(msg){ window.chrome.webview.postMessage(msg); }
    function esc(v){ return String(v ?? '').replace(/[&<>"']/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[m])); }
    function truncate(t,n){ t=String(t||''); return t.length>n ? t.slice(0,n)+'…' : t; }
    function requestRefresh(withSpinner){ setRefreshing(withSpinner); post({type:'refresh'}); }
    function setRefreshing(v){ sync.innerHTML = v ? '<span class="spin"></span>Güncelleniyor...' : 'Canlı takip: aktif'; }
    function install(appId){ if (installingIds[appId]) return; installingIds[appId]=true; render(); post({type:'install', appId}); }

    function renderStore(nextApps){
      apps = Array.isArray(nextApps) ? nextApps : [];
      lastRefreshAt = new Date();
      sync.textContent = 'Son güncelleme: ' + lastRefreshAt.toLocaleTimeString('tr-TR');
      Object.keys(installingIds).forEach(k => delete installingIds[k]);
      render();
    }

    function render(){
      const term = q.value.trim().toLowerCase();
        const list = apps.filter(a => {
        const hay = ((a.display_name||'')+' '+(a.description||'')+' '+(a.category||'')).toLowerCase();
        return !term || hay.includes(term);
      });
      count.textContent = list.length + ' uygulama';
      cards.innerHTML = list.map(a => {
        const installed = !!a.installed;
        const stateFromServer = String(a.install_state || '').toLowerCase();
        if (installed) delete transientState[a.id];
        const state = stateFromServer || String(transientState[a.id] || '').toLowerCase();
        const showInstalled = installed || state === 'installed';
        const inProgress = ['pending','downloading','installing'].includes(state);
        const failMessage = String(a.error_message || '').trim();
        const conflictDetected = !!a.conflict_detected;
        const conflictMessage = String(a.conflict_message || '').trim();
        const requestPending = !!installingIds[a.id];
        const installBtn = showInstalled
          ? `<button class="installed-btn" disabled>✓ Yüklü</button>`
          : inProgress
            ? `<span class="pill ${state}">${stateLabel(state)}</span>`
            : requestPending
              ? `<button disabled>Gönderiliyor...</button>`
              : `<button onclick="install(${a.id})">Kur</button>`;
        const icon = (a.icon_url && String(a.icon_url).trim().length)
          ? `<img class="icon" src="${esc(a.icon_url)}" onerror="this.outerHTML='<div class=&quot;icon icon-fallback&quot;>APP</div>'" />`
          : `<div class="icon icon-fallback">APP</div>`;
        return `<article class="card">
          <div class="row">
            <div class="left">
              ${icon}
              <div>
                <div class="name">${esc(a.display_name || ('App #' + a.id))}</div>
                <div class="meta">v${esc(a.version || '-')} · ${esc(a.file_size_mb || 0)} MB</div>
              </div>
            </div>
            <div>${a.category ? `<span class="pill">${esc(a.category)}</span>` : ''}${installBtn}</div>
          </div>
          <div class="desc">${esc(truncate(a.description || '', 120))}</div>
          ${!showInstalled && conflictDetected && conflictMessage ? `<div class="warn">Uyarı: ${esc(truncate(conflictMessage, 110))}<details><summary>Detay</summary>${esc(conflictMessage)}</details></div>` : ''}
          ${state === 'failed' && failMessage ? `<div class="err">Başarısız: ${esc(truncate(failMessage, 110))}<details><summary>Detay</summary>${esc(failMessage)}</details></div>` : ''}
        </article>`;
      }).join('');
      if (!list.length) {
        cards.innerHTML = '<div class="card empty">Filtreye uygun uygulama yok.</div>';
      }
    }

    let toastTimer = null;
    function applyInstallAck(appId, queueStatus){
      const status = String(queueStatus || '').toLowerCase();
      delete installingIds[appId];
      if (status === 'queued' || status === 'already_queued') transientState[appId] = 'pending';
      else if (status === 'already_installed') transientState[appId] = 'installed';
      else if (status === 'pending' || status === 'downloading' || status === 'installing') transientState[appId] = status;
      render();
    }
    function stateLabel(state){
      if (state === 'pending') return 'Kuyrukta';
      if (state === 'downloading') return 'İndiriliyor';
      if (state === 'installing') return 'Kuruluyor';
      if (state === 'failed') return 'Başarısız';
      return 'Bekleniyor';
    }
    function showToast(message){
      const t = document.getElementById('toast');
      t.textContent = message;
      t.style.display = 'block';
      clearTimeout(toastTimer);
      toastTimer = setTimeout(()=>{ t.style.display='none'; }, 2500);
    }

    // Initial pull
    requestRefresh(true);
    setInterval(()=>requestRefresh(false), 8000);
    window.renderStore = renderStore;
    window.showToast = showToast;
    window.applyInstallAck = applyInstallAck;
  </script>
</body>
</html>
""";
}

internal sealed class StoreBackend
{
    public List<StoreApp> FetchStoreApps()
    {
        var run = RunCli("get_store");
        if (run.ExitCode != 0)
        {
            throw new InvalidOperationException((run.StdErr + "\n" + run.StdOut).Trim());
        }

        var store = JsonSerializer.Deserialize<IpcStoreResponse>(run.StdOut, JsonOpts())
                    ?? throw new InvalidOperationException("Store response is empty");
        if (!string.Equals(store.Status, "ok", StringComparison.OrdinalIgnoreCase))
        {
            throw new InvalidOperationException(string.IsNullOrWhiteSpace(store.Message) ? "Store status is not ok" : store.Message);
        }
        var apps = store.Data?.Apps ?? new List<StoreApp>();
        NormalizeIconUrls(apps);
        return apps;
    }

    public InstallAck RequestInstall(int appId)
    {
        var run = RunCli($"install_from_store {appId}");
        if (run.ExitCode != 0)
        {
            var err = string.Join("\n", new[] { run.StdErr, run.StdOut }.Where(s => !string.IsNullOrWhiteSpace(s))).Trim();
            var msg = err.Length == 0 ? "Kurulum isteği başarısız." : err;
            return new InstallAck("", msg);
        }

        var raw = run.StdOut.Trim();
        if (raw.Length == 0)
        {
            return new InstallAck("", "Kurulum isteği gönderildi.");
        }
        try
        {
            var resp = JsonSerializer.Deserialize<IpcInstallResponse>(raw, JsonOpts());
            var q = (resp?.Data?.QueueStatus ?? "").Trim().ToLowerInvariant();
            var msg = q switch
            {
                "queued" => "Kurulum isteği kuyruğa alındı.",
                "already_queued" => "Bu uygulama zaten kurulum kuyruğunda.",
                "already_installed" => "Bu uygulama zaten yüklü.",
                _ => string.IsNullOrWhiteSpace(resp?.Message) ? "Kurulum isteği gönderildi." : resp!.Message,
            };
            return new InstallAck(q, msg);
        }
        catch
        {
            return new InstallAck("", raw);
        }
    }

    private static CliRunResult RunCli(string args)
    {
        var exePath = Path.Combine(AppContext.BaseDirectory, "appcenter-tray-cli.exe");
        if (!File.Exists(exePath))
        {
            throw new FileNotFoundException("appcenter-tray-cli.exe not found", exePath);
        }

        var psi = new ProcessStartInfo
        {
            FileName = exePath,
            Arguments = args,
            UseShellExecute = false,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            CreateNoWindow = true,
        };

        using var p = Process.Start(psi) ?? throw new InvalidOperationException("Failed to start tray CLI");
        var stdout = p.StandardOutput.ReadToEnd();
        var stderr = p.StandardError.ReadToEnd();
        p.WaitForExit();
        return new CliRunResult(p.ExitCode, stdout, stderr);
    }

    private static JsonSerializerOptions JsonOpts() => new() { PropertyNameCaseInsensitive = true };

    private static void NormalizeIconUrls(List<StoreApp> apps)
    {
        var baseUrl = ReadServerBaseUrl();
        foreach (var app in apps)
        {
            if (string.IsNullOrWhiteSpace(app.IconUrl))
            {
                continue;
            }
            var icon = app.IconUrl.Trim();
            if (icon.StartsWith("http://", true, CultureInfo.InvariantCulture) ||
                icon.StartsWith("https://", true, CultureInfo.InvariantCulture))
            {
                continue;
            }
            if (icon.StartsWith("/") && !string.IsNullOrWhiteSpace(baseUrl))
            {
                app.IconUrl = baseUrl + icon;
            }
        }
    }

    private static string ReadServerBaseUrl()
    {
        try
        {
            var cfgPath = @"C:\ProgramData\AppCenter\config.yaml";
            if (!File.Exists(cfgPath))
            {
                return "";
            }
            foreach (var raw in File.ReadLines(cfgPath))
            {
                var line = raw.Trim();
                if (!line.StartsWith("url:", StringComparison.OrdinalIgnoreCase))
                {
                    continue;
                }
                var value = line[(line.IndexOf(':') + 1)..].Trim().Trim('"', '\'');
                if (value.Length == 0)
                {
                    continue;
                }
                return value.TrimEnd('/');
            }
        }
        catch (Exception ex)
        {
            StoreLog.Write("ReadServerBaseUrl failed: " + ex.Message);
        }
        return "";
    }
}

internal sealed class WebMessage
{
    [JsonPropertyName("type")]
    public string Type { get; set; } = "";

    [JsonPropertyName("appId")]
    public int AppId { get; set; }
}

internal sealed record CliRunResult(int ExitCode, string StdOut, string StdErr);

internal sealed class IpcStoreResponse
{
    public string Status { get; set; } = "";
    public string Message { get; set; } = "";
    public StorePayload? Data { get; set; }
}

internal sealed class IpcInstallResponse
{
    public string Status { get; set; } = "";
    public string Message { get; set; } = "";
    public IpcInstallData? Data { get; set; }
}

internal sealed class IpcInstallData
{
    [JsonPropertyName("queue_status")]
    public string QueueStatus { get; set; } = "";
}

internal sealed record InstallAck(string QueueStatus, string Message);

internal sealed class StorePayload
{
    [JsonPropertyName("apps")]
    public List<StoreApp> Apps { get; set; } = new();
}

internal sealed class StoreApp
{
    [JsonPropertyName("id")]
    public int Id { get; set; }

    [JsonPropertyName("display_name")]
    public string DisplayName { get; set; } = "";

    [JsonPropertyName("version")]
    public string Version { get; set; } = "";

    [JsonPropertyName("description")]
    public string Description { get; set; } = "";

    [JsonPropertyName("category")]
    public string Category { get; set; } = "";

    [JsonPropertyName("file_size_mb")]
    public int FileSizeMB { get; set; }

    [JsonPropertyName("icon_url")]
    public string IconUrl { get; set; } = "";

    [JsonPropertyName("installed")]
    public bool Installed { get; set; }

    [JsonPropertyName("install_state")]
    public string InstallState { get; set; } = "";

    [JsonPropertyName("error_message")]
    public string ErrorMessage { get; set; } = "";

    [JsonPropertyName("conflict_detected")]
    public bool ConflictDetected { get; set; }

    [JsonPropertyName("conflict_confidence")]
    public string ConflictConfidence { get; set; } = "";

    [JsonPropertyName("conflict_message")]
    public string ConflictMessage { get; set; } = "";
}

internal static class StoreLog
{
    private static readonly object Sync = new();

    public static void Write(string message)
    {
        try
        {
            var dir = Path.Combine(
                Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
                "AppCenter",
                "StoreUI");
            Directory.CreateDirectory(dir);
            var path = Path.Combine(dir, "store-ui.log");
            lock (Sync)
            {
                File.AppendAllText(path, $"{DateTime.Now:yyyy-MM-dd HH:mm:ss} {message}{Environment.NewLine}");
            }
        }
        catch
        {
            // best-effort logging only
        }
    }
}
