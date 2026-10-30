export function mediaDate(item) {
 if(item.taken){const value=typeof item.taken==='number'?item.taken:Date.parse(item.taken);if(Number.isFinite(value)&&value>0)return value;}
 return typeof item.modified==='number'?item.modified*1000:Date.parse(item.modified);
}
export function dateRange(filters) {
 const parse=(value,next=false)=>{const [y,m,d]=value.split('-').map(Number);return new Date(y,m-1,d+(next?1:0)).getTime();};
 const start=filters['date-from']?parse(filters['date-from']):-Infinity,end=filters['date-to']?parse(filters['date-to'],true):Infinity;
 if(start>=end)throw new Error('Choose an end date on or after the start date.');return {start,end};
}
export function withinDates(item,range){if(range.start===-Infinity&&range.end===Infinity)return true;const date=mediaDate(item);return Number.isFinite(date)&&date>=range.start&&date<range.end;}
export function matchesFormat(item,format){return !format||format==='all'||item.name.toLowerCase().split('.').at(-1)===format;}
export function filterPhone(items,f){
 const range=dateRange(f),min=f.min===''?0:Number(f.min)*1e6,max=f.max===''?Infinity:Number(f.max)*1e6,q=f.search.trim().toLowerCase();
 return items.filter(i=>i.size>=min&&i.size<=max&&(f.type==='all'||i.type===f.type)&&(f['status-filter']==='all'||(f['status-filter']==='pending'?['pending','hashing'].includes(i.status):i.status===f['status-filter']))&&matchesFormat(i,f.format)&&withinDates(i,range)&&(!q||((i.album||'')+i.name).toLowerCase().includes(q))).sort((a,b)=>{
  if(f.sort==='oldest')return mediaDate(a)-mediaDate(b)||a.name.localeCompare(b.name);
  if(f.sort==='name')return a.name.localeCompare(b.name);
  if(f.sort==='size')return b.size-a.size;
  if(f.sort==='smallest')return a.size-b.size;
  return mediaDate(b)-mediaDate(a)||a.name.localeCompare(b.name);
 });
}
export function mergeLiveRows(store,update,previousId){
 if(update.id!==previousId)store.clear();
 for(const row of update.rows||[]){const old=store.get(row.id);if(!old||row.version>=old.version)store.set(row.id,row);}
 return {id:update.id,cursor:update.cursor,complete:!!update.complete};
}
export function selectablePhone(row,action,fresh){return action==='remove'?row.status==='uploaded':!!fresh&&!row.provisional&&row.status==='missing';}
export function extraCloudFilters(groups,f){
 const range=dateRange(f);const filtered=groups.filter(g=>g.files.some(i=>matchesFormat(i,f.format)&&withinDates(i,range)));
 if(f.sort==='newest'||f.sort==='oldest')filtered.sort((a,b)=>{const date=g=>Math.max(...g.files.map(mediaDate).filter(Number.isFinite),0);return f.sort==='newest'?date(b)-date(a):date(a)-date(b);});
 if(f.sort==='smallest')filtered.sort((a,b)=>a.size-b.size);
 return filtered;
}
