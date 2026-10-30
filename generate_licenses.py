"""Collect notices for Go modules actually linked into the Android engine."""
from pathlib import Path
import html
import os
import subprocess

root = Path(__file__).resolve().parent
go = Path(os.environ['LOCALAPPDATA']) / 'OneDrive Gallery Build/go/bin/go.exe'
lines = subprocess.check_output([str(go), 'list', '-deps', '-f', '{{if .Module}}{{.Module.Path}}|{{.Module.Dir}}{{end}}', '.'], cwd=root/'engine', text=True).splitlines()
modules = dict(line.split('|', 1) for line in lines if '|' in line and not line.startswith('onedrivegallery|'))
sections = []
missing = []
for name, directory in sorted(modules.items()):
    folder = Path(directory)
    licenses = [p for p in folder.iterdir() if p.is_file() and (p.name.upper().startswith(('LICENSE', 'COPYING', 'NOTICE', 'LICENCE')))]
    if not licenses:
        missing.append(name)
    sections.append('<details><summary>'+html.escape(name)+'</summary>'+''.join('<h3>'+html.escape(p.name)+'</h3><pre>'+html.escape(p.read_text(encoding='utf-8', errors='replace'))+'</pre>' for p in licenses)+'</details>')
goroot = Path(subprocess.check_output([str(go), 'env', 'GOROOT'], text=True).strip())
sections.append('<details><summary>Go standard library</summary><pre>'+html.escape((goroot/'LICENSE').read_text())+'</pre></details>')
page = '''<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="style.css"><title>About OneDrive Gallery</title></head><body><main><h1>OneDrive Gallery 1.0</h1><p>Personal app using the open source Rclone OneDrive connector. Independent of Microsoft and Google.</p><p>Microsoft sign-in happens in your external browser. Tokens stay in Android app-private storage, with backups and device transfer disabled. No analytics. OneDrive receives cloud API requests, thumbnail requests, and only the original files you confirm for upload.</p><p>Scans enumerate local Android MediaStore photos/videos. Google cloud-only items are excluded. Google backup status for local files is unavailable. Vault contents and shared shortcuts are excluded from OneDrive scans.</p><p>Checksum equality identifies duplicate candidates, not a cryptographic guarantee or a visual-quality score. Review paths before recycling copies.</p><h2>Open source notices</h2>'''+''.join(sections)+'''</main></body></html>'''
page=page.replace('OneDrive Gallery 1.0','OneDrive Gallery 1.1').replace('<h2>Open source notices','<p>Cloud metadata checkpoints and local original hashes stay in private storage. Removing a phone original requires fresh checksum verification against an existing OneDrive copy and Android deletion confirmation. Playing cloud videos streams media data; PDF and Office previews open OneDrive in your browser.</p><h2>Open source notices')
(root/'app/src/main/assets/licenses.html').write_text(page, encoding='utf-8')
print(f'Collected notices for {len(modules)} linked Go modules and the Go standard library.')
if missing:
    raise SystemExit('Missing notices: '+', '.join(missing))
