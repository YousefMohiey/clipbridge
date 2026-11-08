# ClipBridge client for Windows.
# Keeps the Windows clipboard and the Linux host clipboard in sync, text and images.
# Run at logon: keep this window open, or drop a shortcut to it in shell:startup.

param(
    [string]$Server = "192.168.1.8",
    [int]$Port = 1717,
    [int]$IntervalMs = 400
)

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$logDir = Join-Path $env:LOCALAPPDATA "ClipBridge"
New-Item -ItemType Directory -Force $logDir | Out-Null
try { Start-Transcript -Path (Join-Path $logDir "clipbridge.log") -Append | Out-Null } catch { }

$base = "http://${Server}:$Port"
$lastHash = ""
$lastRev = -1

function Get-Clip {
    try {
        if ([System.Windows.Forms.Clipboard]::ContainsImage()) {
            $img = [System.Windows.Forms.Clipboard]::GetImage()
            if ($img) {
                $ms = New-Object System.IO.MemoryStream
                $img.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
                return @{ type = "image"; bytes = $ms.ToArray(); text = "" }
            }
        }
        if ([System.Windows.Forms.Clipboard]::ContainsText()) {
            $txt = [System.Windows.Forms.Clipboard]::GetText()
            if ($txt) { return @{ type = "text"; bytes = $null; text = $txt } }
        }
    } catch { }
    return $null
}

function Get-Hash($clip) {
    $sha = [System.Security.Cryptography.SHA256]::Create()
    if ($clip.type -eq "image") {
        return ([BitConverter]::ToString($sha.ComputeHash($clip.bytes)) -replace "-", "")
    }
    $b = [System.Text.Encoding]::UTF8.GetBytes($clip.text)
    return ([BitConverter]::ToString($sha.ComputeHash($b)) -replace "-", "")
}

function Push-Clip($clip) {
    if ($clip.type -eq "image") {
        Invoke-RestMethod -Uri "$base/set" -Method Post -Body $clip.bytes -ContentType "image/png" -TimeoutSec 10 | Out-Null
    } else {
        $body = @{ text = $clip.text } | ConvertTo-Json -Compress
        Invoke-RestMethod -Uri "$base/set" -Method Post -Body $body -ContentType "application/json; charset=utf-8" -TimeoutSec 10 | Out-Null
    }
}

Write-Host "ClipBridge -> $base  (Ctrl+C to stop)"

while ($true) {
    try {
        $clip = Get-Clip
        if ($clip) {
            $h = Get-Hash $clip
            if ($h -ne $lastHash) {
                Push-Clip $clip
                $lastHash = $h
            }
        }
    } catch { }

    try {
        $st = Invoke-RestMethod -Uri "$base/state" -TimeoutSec 5
        if ($st.type -and $st.rev -ne $lastRev -and $st.hash -ne $lastHash) {
            if ($st.type -eq "image") {
                $bytes = (New-Object System.Net.WebClient).DownloadData("$base/image")
                $ms = New-Object System.IO.MemoryStream(, $bytes)
                $img = [System.Drawing.Image]::FromStream($ms)
                [System.Windows.Forms.Clipboard]::SetImage($img)
            } elseif ($st.type -eq "text") {
                [System.Windows.Forms.Clipboard]::SetText([string]$st.text)
            }
            $lastHash = [string]$st.hash
            $lastRev = [int]$st.rev
            Write-Host ("applied {0} from host" -f $st.type)
        } elseif ($st.rev -ne $lastRev) {
            $lastRev = [int]$st.rev
        }
    } catch { }

    Start-Sleep -Milliseconds $IntervalMs
}
