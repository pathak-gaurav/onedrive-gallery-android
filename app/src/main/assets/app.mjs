import {bytes,filterGroups,markExtras,selectFile,selectionPlans} from './model.mjs';
import {mediaDate,filterPhone,extraCloudFilters,mergeLiveRows,selectablePhone} from './gallery-model.mjs';
const $=id=>document.getElementById(id),pending=new Map();let seq=0;
function api(op,body={}){return new Promise((resolve,reject)=>{const id=String(++seq);pending.set(id,{resolve,reject});if(!window.Native){pending.delete(id);reject(new Error('This screen runs inside the Android app.'));return;}window.Native.request(id,op,JSON.stringify(body));});}
window.nativeResult=(id,json)=>{const promise=pending.get(String(id));if(!promise)return;pending.delete(String(id));try{const data=JSON.parse(json);if(data.error)promise.reject(new Error(data.error));else promise.resolve(data);}catch(e){promise.reject(e);}};
const s={mode:'all',reports:{},queues:{all:new Map(),folder:new Map()},media:[],mediaMap:new Map(),phoneSelected:new Set(),page:0,folder:[],busy:false,signedIn:false,permissions:{},watching:false,authURL:'',jobHandled:'',filters:{},phoneFresh:false,streamId:0,cursor:0,liveInfo:{},phoneCards:new Map(),confirming:false};
const filterIds=['search','min','max','type','sort','status-filter','date-from','date-to','format'];
const node=(tag,cls,text)=>{const n=document.createElement(tag);if(cls)n.className=cls;if(text!==undefined)n.textContent=text;return n;};
const message=(text,error=false)=>{$('notice').textContent=text;$('notice').classList.toggle('error',error);};
const guarded=fn=>async(...args)=>{try{await fn(...args);}catch(e){message(e.message,true);}};
const report=()=>s.reports[s.mode];
const removing=()=>s.mode==='phone'&&$('phone-action').value==='remove';
const canInterrupt=()=>removing()&&s.jobOperation==='compare';
function currentFolder(){return s.folder.at(-1)||{id:'',path:''};}
function setBusy(busy){s.busy=busy;for(const id of ['login','rescan','compare','scan-folder','mark','review','permission','folder-up','browse-upload','reconnect'])$(id).disabled=busy;$('mark').disabled=busy&&!canInterrupt();for(const b of $('folders').querySelectorAll('button'))b.disabled=busy;for(const box of $('results').querySelectorAll('input[type=checkbox]')){const row=s.mediaMap.get(box.dataset.file);box.disabled=(busy&&!canInterrupt())||(s.mode==='phone'&&(!row||!selectablePhone(row,$('phone-action').value,s.phoneFresh)));}updateSelection();}
function filters(){return Object.fromEntries(filterIds.map(id=>[id,$(id).value]));}
function filtered(){
 const f=filters();if(Number(f.min)<0||Number(f.max)<0||f.min!==''&&f.max!==''&&Number(f.min)>Number(f.max))throw new Error('Use a maximum size at least as large as the minimum.');
 if(s.mode!=='phone')return extraCloudFilters(filterGroups(report()?.groups||[],{...f,copies:'2'}),f);
 return filterPhone(s.media,f);
}
function updateSelection(){
 const phone=s.mode==='phone';let n=0;
 if(phone)n=s.phoneSelected.size;else for(const ids of s.queues[s.mode].values())n+=ids.size;
 $('review').textContent=phone?(removing()?`Remove from phone (${n})`:`Upload selected (${n})`):`Review deletion (${n})`;
 $('review').className=phone&&!removing()?'primary':'danger';$('review').disabled=(s.busy&&!canInterrupt())||s.confirming||!n||phone&&!removing()&&!s.phoneFresh||!phone&&!report()?.fresh;
 $('clear').disabled=(s.busy&&!canInterrupt())||!n;$('selection-note').textContent=`${n} ${phone?'local files':'cloud copies'} selected across filters in this tab.`;
}
function copyCard(file,group,reuse){
 const phone=!group,card=node('article','copy');const selected=phone?s.phoneSelected.has(file.id):s.queues[s.mode].get(group.id)?.has(file.id);card.classList.toggle('selected',!!selected);
 const preview=reuse||node('button','preview');preview.type='button';preview.setAttribute('aria-label',`${file.type==='video'?'Play':'Preview'} ${phone?file.name:file.path}`);preview.onclick=guarded(()=>api('preview',{source:phone?'phone':'cloud',id:file.id}));
 if(!reuse){const image=node('img');image.loading='lazy';image.alt=file.name;image.src=`/thumb/${phone?'phone':'cloud'}?id=${encodeURIComponent(file.id)}`;image.addEventListener('error',()=>{image.remove();preview.append(node('span','placeholder',file.type==='video'?'▷':'▧'));},{once:true});preview.append(image);}card.append(preview);
 const body=node('div','copy-body'),label=node('label');const check=node('input');check.type='checkbox';check.dataset.file=file.id;check.setAttribute('aria-label',`${phone?(removing()?'Remove from phone':'Upload'):'Delete'} ${phone?file.album+file.name:file.path}`);check.checked=!!selected;check.disabled=(s.busy&&!canInterrupt())||(phone&&!selectablePhone(file,$('phone-action').value,s.phoneFresh));
 check.onchange=()=>{if(phone){if(check.checked&&removing()&&s.phoneSelected.size>=100){check.checked=false;message('Up to 100 phone originals per deletion batch.',true);return;}if(check.checked)s.phoneSelected.add(file.id);else s.phoneSelected.delete(file.id);}else if(!selectFile(group,s.queues[s.mode],file.id,check.checked)){check.checked=false;message('Keep at least one copy in every group.',true);}card.classList.toggle('selected',check.checked);updateSelection();};
 label.append(check,document.createTextNode(phone?(removing()?'Remove phone copy':'Select for upload'):'Select to delete'));body.append(label);
 if(phone){const status={uploaded:file.provisional?'Match found · checking changes':'Already uploaded',missing:'Not uploaded',unknown:'Needs review',pending:'Checking OneDrive…',hashing:'Reading original…'}[file.status]||file.status;body.append(node('p','badge '+file.status,status));}
 body.append(node('p','path',phone?(file.album||'')+file.name:file.path));
 const dims=file.width&&file.height?`${file.width} × ${file.height} · `:'';const date=new Date(mediaDate(file));body.append(node('div','metadata',`${bytes(file.size)} · ${dims}${file.taken?'Taken':'Modified'} ${date.toLocaleString()}${file.duration?' · '+Math.round(file.duration/1000)+' sec':''}`));
 const openPreview=node('button','',file.type==='video'?'▶ Play video':'Open preview');openPreview.onclick=guarded(()=>api('preview',{source:phone?'phone':'cloud',id:file.id}));body.append(openPreview);
 if(phone){body.append(node('small','', 'On this phone'));if(file.message)body.append(node('p','metadata',file.message));for(const match of file.matches||[]){const b=node('button','',`OneDrive ↗ ${match.path}`);b.onclick=guarded(()=>api('open',{url:match.webUrl}));body.append(b);}}
 else {const open=node('button','','OneDrive ↗');open.onclick=guarded(()=>api('open',{url:file.webUrl}));const keep=node('button','','Keep this copy; mark others');keep.disabled=s.busy;keep.onclick=()=>{s.queues[s.mode].set(group.id,new Set(group.files.filter(f=>f.id!==file.id).map(f=>f.id)));render();};body.append(open,keep);}
 card.append(body);return card;
}
function render(){
 const items=filtered(),phone=s.mode==='phone',pageSize=phone?60:12;const max=Math.max(1,Math.ceil(items.length/pageSize));s.page=Math.min(s.page,max-1);const result=$('results');result.replaceChildren();
 if(!items.length)result.append(node('div','empty',phone?(s.media.length?'No local files match these filters.':'Compare your phone to see what’s already uploaded.'):(report()?'No duplicate groups found in this scan.':'Start a scan to find duplicate candidates.')));
 const visible=items.slice(s.page*pageSize,(s.page+1)*pageSize);
 if(phone){const grid=node('div','phone-grid'),next=new Map();for(const file of visible){const key=`${file.version}:${s.phoneSelected.has(file.id)}:${s.busy}:${s.phoneFresh}:${$('phone-action').value}`,old=s.phoneCards.get(file.id);const card=old?.key===key?old.card:copyCard(file,null,old?.card.querySelector('.preview'));next.set(file.id,{key,card});grid.append(card);}s.phoneCards=next;result.append(grid);$('counts').textContent=`${items.length.toLocaleString()} results · ${s.media.length.toLocaleString()} on this phone`;renderChips();}
 else {for(const g of visible){const panel=node('section','group'),heading=node('div','group-heading');heading.append(node('h3','',g.files[0].name),node('span','',`${g.files.length} copies · ${bytes(g.savings)} extra`));const copies=node('div','copies');g.files.forEach(f=>copies.append(copyCard(f,g)));panel.append(heading,copies);result.append(panel);}$('counts').textContent=`${items.length} groups · ${bytes(items.reduce((sum,g)=>sum+g.savings,0))} potential recovery`;}
 $('pagination').hidden=items.length<=pageSize;$('page-count').textContent=`${s.page+1} / ${max}`;$('previous').disabled=s.page===0;$('next').disabled=s.page+1>=max;updateSelection();
}
async function browse(){const f=currentFolder();$('folder-path').textContent=f.path||'OneDrive';$('folder-up').disabled=s.busy||s.folder.length===0;$('folders').replaceChildren(node('p','muted','Loading folders…'));const data=await api('folders',{id:f.id});$('folders').replaceChildren();for(const dir of data.folders){const b=node('button','',`▱ ${dir.name}`);b.onclick=guarded(async()=>{s.folder.push({...dir,path:[f.path,dir.name].filter(Boolean).join('/')});await browse();});$('folders').append(b);}if(!data.folders.length)$('folders').append(node('p','muted','No subfolders. You can scan this folder.'));}
async function changeMode(mode){
 s.filters[s.mode]=filters();s.mode=mode;s.page=0;populateFormats();for(const id of filterIds)if(s.filters[mode])$(id).value=s.filters[mode][id];else $(id).value=defaultFilter(id);
 for(const m of ['all','folder','phone'])$(`tab-${m}`).setAttribute('aria-selected',String(mode===m));$('folder-controls').hidden=mode!=='folder';$('phone-controls').hidden=mode!=='phone';$('cloud-filters').hidden=mode==='phone';$('phone-filter').hidden=mode!=='phone';
 $('status-chips').hidden=mode!=='phone';$('phone-action-wrap').hidden=mode!=='phone';$('view-title').textContent=mode==='phone'?'Your phone library':mode==='folder'?'Folder duplicates':'Duplicate files';$('rescan').textContent=mode==='phone'?'Continue / refresh':mode==='folder'?'Rescan folder':'Scan OneDrive';$('mark').textContent=mode==='phone'?'Select filtered files':'Mark filtered extra copies';setScope();render();if(mode==='folder')await browse();
}
function defaultFilter(id){return ['search','min','max','date-from','date-to'].includes(id)?'':id==='sort'?(s.mode==='phone'?'newest':'savings'):'all';}
function populateFormats(){const current=$('format').value;const files=s.mode==='phone'?s.media:(report()?.groups||[]).flatMap(g=>g.files);const extensions=[...new Set(files.map(f=>f.name.includes('.')?f.name.split('.').at(-1).toLowerCase():'').filter(Boolean))].sort();$('format').replaceChildren(new Option('All formats','all'));for(const ext of extensions)$('format').append(new Option(ext.toUpperCase(),ext));$('format').value=extensions.includes(current)?current:'all';}
async function chooseUploadFolder(){
 const stack=[],dialog=node('dialog'),title=node('h2','','Upload destination'),location=node('p','path','OneDrive'),up=node('button','','↑ Parent'),folders=node('div','folders'),use=node('button','primary','Use this folder'),close=node('button','','Cancel'),actions=node('div','row');actions.append(close,use);dialog.append(title,location,up,folders,actions);document.body.append(dialog);dialog.showModal();dialog.addEventListener('close',()=>dialog.remove(),{once:true});
 let request=0;async function load(){const generation=++request,f=stack.at(-1)||{id:'',path:''};location.textContent=f.path||'OneDrive root';up.disabled=!stack.length;use.disabled=true;folders.replaceChildren(node('p','muted','Loading…'));try{const data=await api('folders',{id:f.id});if(generation!==request)return;folders.replaceChildren();for(const dir of data.folders){const b=node('button','',`▱ ${dir.name}`);b.onclick=guarded(async()=>{stack.push({...dir,path:[f.path,dir.name].filter(Boolean).join('/')});await load();});folders.append(b);}if(!data.folders.length)folders.append(node('p','muted','No subfolders'));use.disabled=false;}catch(e){folders.replaceChildren(node('p','error',e.message));}}
 up.onclick=guarded(async()=>{stack.pop();await load();});close.onclick=()=>dialog.close();use.onclick=()=>{$('upload-folder').value=stack.at(-1)?.path||'';dialog.close();};await load();
}
function setScope(){const r=report();$('scope').textContent=s.mode==='phone'?(s.phoneFresh?'Up to date. Select missing originals to upload, or matched copies to remove from this phone.':s.busy?'Results appear live. Unmatched files stay “Checking” until OneDrive finishes.':'Continue comparison to refresh or resume saved progress.'):r?`${s.mode==='folder'?(r.folderPath||'OneDrive')+' · ':''}${r.fresh?'Updated':'Obsolete; rescan required'} ${new Date(r.scannedAt).toLocaleString()}`:'No completed scan in this tab.';}
function renderChips(){const counts={all:s.media.length,uploaded:0,missing:0,pending:0,unknown:0};for(const f of s.media)counts[f.status==='hashing'?'pending':f.status]++;for(const b of $('status-chips').querySelectorAll('button')){b.querySelector('strong').textContent=(counts[b.dataset.status]||0).toLocaleString();b.setAttribute('aria-pressed',String($('status-filter').value===b.dataset.status));}}
async function drainLive(max=4){let changed=false;for(let i=0;i<max;i++){let update=await api('comparison',{since:s.cursor});if(!update.id)break;if(update.id!==s.streamId){if(s.cursor)update=await api('comparison',{since:0});s.phoneSelected.clear();s.phoneCards.clear();changed=true;}const merged=mergeLiveRows(s.mediaMap,update,s.streamId);s.streamId=merged.id;s.cursor=merged.cursor;s.liveInfo=update;changed=changed||!!update.rows?.length;if(!update.hasMore)break;}if(changed){s.media=[...s.mediaMap.values()];populateFormats();}s.phoneFresh=!!s.liveInfo.complete;return changed;}
async function start(operation,body={}){await api('start',{operation,body});if(['scan','compare','upload','delete','verifyPhoneDelete'].includes(operation))s.phoneFresh=false;s.jobHandled='';s.rewatch=true;setBusy(true);await watch();}
async function poll(){
 const state=await api('status');s.permissions=state.permissions;s.signedIn=state.signedIn;$('connect').hidden=s.signedIn;$('connected').hidden=!s.signedIn;$('reconnect').hidden=!s.signedIn;
 $('permission-note').textContent=state.permissions.full&&state.permissions.originals?'All local photos and videos allowed.':'Allow all photos/videos and original metadata for a complete comparison.';$('permission').textContent=state.permissions.full?'Manage access':'Allow gallery access';
 const login=state.login;if(login?.status==='running'&&login.url){$('reopen-login').hidden=false;if(s.authURL!==login.url){s.authURL=login.url;await api('openLogin');}}
 return state;
}
async function watch(){
 if(s.watching)return;s.watching=true;
 try{while(true){const state=await poll(),job=state.job;s.jobOperation=job.operation;const running=job.status==='running';setBusy(running);$('activity').hidden=!running;
  if(state.comparison?.id&&(state.comparison.id!==s.streamId||state.comparison.version>s.cursor)){const changed=await drainLive();if(changed&&s.mode==='phone')render();}
  const scan=state.scan||{};$('scan-stats').hidden=!['scan','compare','upload','delete','verifyPhoneDelete'].includes(job.operation);$('cloud-count').textContent=(scan.files||0).toLocaleString();$('local-count').textContent=(state.comparison?.counts?.uploaded||0).toLocaleString();$('page-progress').textContent=`${scan.pages||0} cloud pages · ${scan.resuming?'resuming checkpoint':scan.incremental?'checking changes':'first scan'}`;
  if(job.deletionDone)document.getElementById('review-dialog')?.close();
  if(running){$('activity-message').textContent=job.message;setScope();if(job.total&&scan.phase==='complete'){$('progress').max=job.total;$('progress').value=job.done;}else $('progress').removeAttribute('value');await new Promise(r=>setTimeout(r,1000));continue;}
  const fingerprint=JSON.stringify(job);if((job.status==='complete'||job.status==='error')&&s.jobHandled!==fingerprint){s.jobHandled=fingerprint;if(job.status==='error'){document.getElementById('review-dialog')?.close();message(job.message,true);}else await completed(job.operation,await api('result'));}if(s.rewatch){s.rewatch=false;continue;}break;
 }}finally{s.watching=false;}
}
async function completed(operation,result){
 document.getElementById('review-dialog')?.close();
 if(operation==='scan'){const mode=result.mode;s.reports[mode]={...result.report,fresh:true};s.queues[mode].clear();message(`Fresh scan complete: ${result.fileCount} files checked.`);}
 if(operation==='login'){s.reports={};s.media=[];s.phoneFresh=false;s.phoneSelected.clear();s.queues.all.clear();s.queues.folder.clear();message('Connected to OneDrive.');$('reopen-login').hidden=true;}
 if(operation==='compare'){await drainLive(1000);s.phoneFresh=!!result.complete;s.phoneSelected.clear();message(`${result.total} local files compared.${result.fullAccess?'':' Limited gallery access; allow all to include the entire library.'}`);}
 if(operation==='verifyPhoneDelete'){s.confirming=true;message(`${result.count} originals verified in OneDrive. Review Android’s deletion confirmation.`);try{await api('confirmPhoneDelete',{token:result.token});}catch(e){s.confirming=false;throw e;}}
 if(operation==='delete'){
  const mode=result.mode;s.queues[mode].clear();if(result.scan)s.reports[mode]={...result.scan.report,fresh:true};else if(s.reports[mode])s.reports[mode].fresh=false;
  message(`${result.deletion.deleted?.length||0} copies recycled. ${result.scan?'The folder/view has been scanned again.':''}${result.deletion.error||''}${result.refreshError?' Refresh failed: '+result.refreshError:''}`,!!(result.deletion.error||result.refreshError));
 }
 if(operation==='upload'){
  s.phoneSelected.clear();if(result.comparison){await drainLive(1000);s.phoneFresh=!!result.comparison.complete;}else s.phoneFresh=false;
  showUploadResults(result);
 }
 populateFormats();setScope();render();
}
function showUploadResults(result){
 const outcomes=result.outcomes||[], failed=outcomes.filter(o=>o.status==='error'||o.status==='review');
 const count=status=>outcomes.filter(o=>o.status===status).length;
 const unfinished=Math.max(0,(result.requested||outcomes.length)-outcomes.length);
 message(`Upload finished: ${count('uploaded')} uploaded, ${count('skipped')} already in OneDrive, ${failed.length} need attention${unfinished?`, ${unfinished} not attempted`:''}.${result.refreshError?' Comparison refresh failed: '+result.refreshError:''}`,!!result.refreshError);
 if(!failed.length&&!unfinished)return;
 const dialog=node('dialog'),title=node('h2','','Upload results'),description=node('p','','Other files continued. Review the items below; existing OneDrive files were not overwritten.'),list=node('ul','review-list');
 for(const item of failed)list.append(node('li','',`${item.name}: ${item.message||'Needs review'}`));
 if(unfinished)list.append(node('li','',`${unfinished} files were not attempted. Select them again after refreshing.`));
 const choice=node('label','','For filename conflicts'),policy=node('select');for(const [value,label] of [['fail','Skip and review conflicts'],['rename','Upload a separate copy with a new name']]){const option=node('option','',label);option.value=value;policy.append(option);}policy.value=$('upload-conflict').value;choice.append(policy);const actions=node('div','row'),close=node('button','','Ignore and close'),retry=node('button','primary','Retry eligible files');actions.append(close,retry);dialog.append(title,description,list,choice,actions);document.body.append(dialog);dialog.showModal();dialog.addEventListener('close',()=>dialog.remove(),{once:true});close.onclick=()=>dialog.close();
 const eligible=failed.filter(o=>o.status==='error'&&s.mediaMap.get(o.id)?.status==='missing');retry.disabled=!s.phoneFresh||!eligible.length;
 if(retry.disabled)dialog.append(node('p','muted','Refresh the comparison and select files again before retrying.'));
 retry.onclick=guarded(async()=>{dialog.close();await start('upload',{ids:eligible.map(o=>o.id),destination:result.destination,preserveAlbums:result.preserveAlbums,conflictBehavior:policy.value});});
}
function review(){
 const phone=s.mode==='phone';const files=phone?s.media.filter(f=>s.phoneSelected.has(f.id)):selectionPlans(report().groups,s.queues[s.mode]).flatMap(p=>p.remove);if(!files.length)return;
 if(phone&&removing()){if(files.length>100){message('Select at most 100 matched originals per phone-deletion batch.',true);return;}return reviewPhoneDeletion(files);}
 const dialog=node('dialog');dialog.id='review-dialog';const title=node('h2','',phone?`Upload ${files.length} originals?`:`Recycle ${files.length} copies?`);dialog.setAttribute('aria-labelledby','review-title');title.id='review-title';
 const text=node('p','',phone?`${bytes(files.reduce((n,f)=>n+f.size,0))} to ${$('upload-folder').value||'OneDrive root'}${$('preserve-albums').checked?', preserving album folders':''}. OneDrive is refreshed first. Already uploaded originals are skipped. Your phone files stay in place.`:'Selected files move to OneDrive’s Recycle Bin and disappear from synced devices. One copy in each group stays. Each file is checked again before deletion.');
 const ul=node('ul','review-list');for(const f of files)ul.append(node('li','selected-line',phone?(f.album||'')+f.name:f.path));
 const status=node('p','muted'),actions=node('div','row'),cancel=node('button','','Cancel'),confirm=node('button',phone?'primary':'danger',phone?'Upload selected originals':'Move to Recycle Bin');actions.append(cancel,confirm);dialog.append(title,text,ul,status,actions);document.body.append(dialog);dialog.showModal();cancel.onclick=()=>dialog.close();dialog.addEventListener('close',()=>dialog.remove(),{once:true});dialog.addEventListener('cancel',e=>{if(s.busy)e.preventDefault();});
 confirm.onclick=guarded(async()=>{
  confirm.disabled=true;cancel.disabled=true;status.textContent=phone?'Uploading, then refreshing the comparison…':'Rechecking and recycling… This window closes when deletion finishes.';
  try{if(phone){s.phoneFresh=false;await start('upload',{ids:files.map(f=>f.id),destination:$('upload-folder').value,preserveAlbums:$('preserve-albums').checked,conflictBehavior:$('upload-conflict').value});}else{const mode=s.mode,r=report();s.deleteMode=mode;r.fresh=false;s.phoneFresh=false;await start('delete',{mode,revision:r.revision,ids:files.map(f=>f.id),folderId:r.folderId,folderPath:r.folderPath});}}catch(e){confirm.disabled=false;cancel.disabled=false;throw e;}
 });
}
function reviewPhoneDeletion(files){
 const dialog=node('dialog'),title=node('h2','',`Remove ${files.length} ${files.length===1?'original':'originals'} from this phone?`),text=node('p','','Each selected original is rehashed and its OneDrive copy is checked again. Android then asks you to confirm deletion from the phone. OneDrive copies stay in place.'),list=node('ul','review-list'),cancel=node('button','','Cancel'),confirm=node('button','danger','Verify backups & continue'),actions=node('div','row');
 for(const file of files)list.append(node('li','',(file.album||'')+file.name));actions.append(cancel,confirm);dialog.append(title,text,list,actions);document.body.append(dialog);dialog.showModal();dialog.addEventListener('close',()=>dialog.remove(),{once:true});cancel.onclick=()=>dialog.close();confirm.onclick=guarded(async()=>{dialog.close();if(s.busy){message('Saving comparison progress before verifying selected originals…');await api('cancel');while(s.watching)await new Promise(resolve=>setTimeout(resolve,100));}s.phoneFresh=false;await start('verifyPhoneDelete',{ids:files.map(f=>f.id)});});
}
for(const mode of ['all','folder','phone'])$(`tab-${mode}`).onclick=guarded(()=>changeMode(mode));
$('login').onclick=guarded(()=>start('login'));$('reopen-login').onclick=guarded(()=>api('openLogin'));$('permission').onclick=guarded(()=>api('permissions'));
$('reconnect').onclick=guarded(()=>start('login'));
$('browse-upload').onclick=guarded(chooseUploadFolder);$('reset-filters').onclick=()=>{for(const id of filterIds)$(id).value=defaultFilter(id);s.page=0;render();};
$('compare').onclick=guarded(async()=>{s.phoneFresh=false;await start('compare');});
$('scan-folder').onclick=guarded(async()=>{s.lastScanMode='folder';const f=currentFolder();if(s.reports.folder)s.reports.folder.fresh=false;await start('scan',{mode:'folder',folderId:f.id,folderPath:f.path});});
$('folder-up').onclick=guarded(async()=>{s.folder.pop();await browse();});
$('rescan').onclick=guarded(async()=>{if(s.mode==='phone'){s.phoneFresh=false;await start('compare');return;}const mode=s.mode;s.lastScanMode=mode;const r=report();if(r)r.fresh=false;const f=r?{id:r.folderId,path:r.folderPath}:currentFolder();await start('scan',{mode,folderId:mode==='folder'?f.id:'',folderPath:mode==='folder'?f.path:''});});
$('mark').onclick=guarded(()=>{const items=filtered();if(s.mode==='phone'){if(!s.phoneFresh&&!removing())throw new Error('Finish comparison before selecting uploads.');for(const f of items)if(selectablePhone(f,$('phone-action').value,s.phoneFresh)&&(!removing()||s.phoneSelected.size<100))s.phoneSelected.add(f.id);}else markExtras(items,s.queues[s.mode],$('keep').value);render();});
$('phone-action').onchange=()=>{s.phoneSelected.clear();$('status-filter').value=removing()?'uploaded':'missing';$('upload-destination').hidden=removing();s.page=0;setBusy(s.busy);render();};
for(const button of $('status-chips').querySelectorAll('button'))button.onclick=()=>{$('status-filter').value=button.dataset.status;s.page=0;render();};
$('clear').onclick=()=>{if(s.mode==='phone')s.phoneSelected.clear();else s.queues[s.mode].clear();render();};$('review').onclick=guarded(review);
for(const id of filterIds)$(id).addEventListener('input',guarded(()=>{s.page=0;render();}));
$('previous').onclick=()=>{s.page--;render();};$('next').onclick=()=>{s.page++;render();};$('stop').onclick=guarded(()=>api('cancel'));
window.appBack=()=>{const d=document.querySelector('dialog[open]');if(d&&!s.busy)d.close();else if(s.mode==='folder'&&s.folder.length&&!s.busy){s.folder.pop();guarded(browse)();}else if(s.mode!=='all')guarded(()=>changeMode('all'))();};
window.appResume=guarded(async()=>{await poll();if(!s.watching)await watch();});
window.phoneDeleteResult=guarded(async removed=>{s.confirming=false;s.phoneSelected.clear();s.phoneFresh=false;s.rewatch=true;message(removed?'Phone copies removed. Refreshing your gallery…':'Phone deletion cancelled. Refreshing the comparison…');setBusy(true);if(!s.watching)await watch();});
guarded(async()=>{await poll();render();await watch();})();
