"""Isolated browser fixture. No OneDrive connection, credentials, uploads, or deletions."""
from http.server import ThreadingHTTPServer, SimpleHTTPRequestHandler
from pathlib import Path
import os

assets=Path(__file__).parent/'app/src/main/assets'
MOCK=r'''
window.Native={request(id,op,json){const body=JSON.parse(json);let value={ok:true};const elapsed=Date.now()-(window.qaStart||0),running=elapsed<18000,active=!!window.qaStart;
const rows=[{id:'phone-a',name:'Garden.jpg',album:'DCIM/Camera/',size:4567890,modified:1793100000,taken:0,type:'image',width:4032,height:3024,status:'uploaded',matches:[{path:'Photos/Garden.jpg',webUrl:'https://onedrive.live.com/'}]},{id:'phone-b',name:'Trip.mp4',album:'Movies/',size:89123456,modified:1793000000,type:'video',width:3840,height:2160,status:running?'pending':'missing',matches:[]},{id:'phone-c',name:'Sunset.jpg',album:'DCIM/Camera/',size:5678901,modified:1792900000,type:'image',width:4032,height:3024,status:'unknown',message:'A same-size cloud file has no comparable checksum',matches:[]},{id:'phone-d',name:'City.jpg',album:'DCIM/Camera/',size:3456789,modified:1792800000,type:'image',width:4032,height:3024,status:running?'pending':'missing',matches:[]}].map((r,i)=>({...r,provisional:running,version:running?i+1:i+5}));
const info={id:active?1:0,version:running?4:8,total:4,complete:!running,fullAccess:true,counts:{uploaded:1,missing:running?0:2,pending:running?2:0,unknown:1}};
if(op==='status')value={signedIn:true,permissions:{full:true,originals:true},login:{status:'idle'},indexComplete:!running,scan:{phase:running?'indexing':'complete',files:active?(running?1500:45290):0,pages:running?3:91,resuming:true},comparison:info,job:active?{status:running?'running':'complete',operation:'compare',message:'Comparing while OneDrive metadata arrives…',done:1,total:4}:{status:'idle'}};
if(op==='start'){window.qaStart=Date.now();value={ok:true};}
if(op==='comparison')value={...info,rows:rows.filter(r=>r.version>(body.since||0)),cursor:running?4:8,hasMore:false};
if(op==='result')value={...info,streamed:true};
if(op==='folders')value={folders:body.id?[]:[{id:'photos',name:'Photos'},{id:'archive',name:'Archive'}]};
if(op==='preview'){document.getElementById('notice').textContent='Preview requested for '+body.id+' (test fixture)';}
if(op==='cancel'){window.qaStart=Date.now()-18001;}
setTimeout(()=>window.nativeResult(id,JSON.stringify(value)),20);
}};
'''
class Handler(SimpleHTTPRequestHandler):
 def do_GET(self):
  path=self.path.split('?')[0]
  if path=='/qa-native.js':data=MOCK.encode();mime='text/javascript'
  elif path.startswith('/thumb/'):
   data=b'<svg xmlns="http://www.w3.org/2000/svg" width="480" height="320"><defs><linearGradient id="g" x2="1" y2="1"><stop stop-color="#aed6bc"/><stop offset="1" stop-color="#3d746c"/></linearGradient></defs><rect width="480" height="320" fill="url(#g)"/><circle cx="370" cy="75" r="37" fill="#f7df98"/><path d="M0 320L145 125L280 320M150 320L330 160L480 320" fill="#285e4f"/></svg>';mime='image/svg+xml'
  else:
   name='index.html' if path=='/' else path.lstrip('/')
   if '/' in name or not (assets/name).is_file():self.send_error(404);return
   data=(assets/name).read_bytes();mime='text/html' if name.endswith('.html') else 'text/css' if name.endswith('.css') else 'text/javascript'
   if name=='index.html':data=data.replace(b'<script type="module"',b'<script src="qa-native.js"></script><script type="module"')
  self.send_response(200);self.send_header('Content-Type',mime);self.send_header('Cache-Control','no-store');self.end_headers();self.wfile.write(data)
 def log_message(self,*args):pass
if __name__=='__main__':
 server=ThreadingHTTPServer(('127.0.0.1',56609),Handler);print('Android UI fixture: http://127.0.0.1:56609/',flush=True);server.serve_forever()
