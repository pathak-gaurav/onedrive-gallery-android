import test from 'node:test';
import assert from 'node:assert/strict';
import {filterPhone,mediaDate,extraCloudFilters,dateRange,mergeLiveRows,selectablePhone} from './app/src/main/assets/gallery-model.mjs';
const f={search:'',type:'all',min:'',max:'',sort:'newest',format:'all','status-filter':'all','date-from':'','date-to':''};
const rows=[{id:'a',name:'old.jpg',album:'DCIM/',type:'image',status:'missing',size:100,taken:new Date(2026,9,1,12).getTime(),modified:1},{id:'b',name:'new.mp4',album:'Movies/',type:'video',status:'uploaded',size:200,taken:new Date(2026,9,2,23,59).getTime(),modified:2},{id:'c',name:'same.jpg',album:'DCIM/',type:'image',status:'unknown',size:300,taken:0,modified:new Date(2026,9,3,0,0).getTime()/1000}];
test('date search includes the full end date and prefers captured date',()=>{
 assert.deepEqual(filterPhone(rows,{...f,'date-from':'2026-10-01','date-to':'2026-10-02'}).map(r=>r.id),['b','a']);
 assert.equal(mediaDate(rows[2]),rows[2].modified*1000);
 assert.throws(()=>dateRange({...f,'date-from':'2026-10-03','date-to':'2026-10-02'}));
});
test('status, type, extension, date and size combine without changing source',()=>{
 assert.deepEqual(filterPhone(rows,{...f,type:'image',format:'jpg','status-filter':'missing',search:'dcim',min:'0.0001',max:'0.0001'}).map(r=>r.id),['a']);
 assert.deepEqual(rows.map(r=>r.id),['a','b','c']);
});
test('phone sorting supports oldest, newest, sizes and names',()=>{
 for(const [sort,expected] of [['oldest','abc'],['newest','cba'],['size','cba'],['smallest','abc'],['name','bac']])assert.equal(filterPhone(rows,{...f,sort}).map(r=>r.id).join(''),expected);
});
test('cloud date filtering retains groups with a matching file',()=>{
 const groups=[{id:'x',size:100,files:[rows[0],rows[1]]},{id:'y',size:300,files:[rows[2]]}];
 assert.deepEqual(extraCloudFilters(groups,{...f,'date-to':'2026-10-02',format:'mp4'}).map(g=>g.id),['x']);
});
test('live batches replace provisional records and reset on a new comparison',()=>{
 const store=new Map();mergeLiveRows(store,{id:1,cursor:1,rows:[{id:'a',version:1,status:'pending'}]},0);
 mergeLiveRows(store,{id:1,cursor:2,rows:[{id:'a',version:2,status:'uploaded'}]},1);
 mergeLiveRows(store,{id:1,cursor:1,rows:[{id:'a',version:1,status:'pending'}]},1);assert.equal(store.get('a').status,'uploaded');
 mergeLiveRows(store,{id:2,cursor:1,rows:[{id:'b',version:1,status:'pending'}]},1);assert.deepEqual([...store.keys()],['b']);
});
test('only positive matches can be selected for removal before final verification',()=>{
 assert.equal(selectablePhone({status:'uploaded',provisional:true},'remove',true),true);
 assert.equal(selectablePhone({status:'uploaded'},'remove',false),true);
 assert.equal(selectablePhone({status:'missing',provisional:true},'upload',true),false);
 assert.equal(selectablePhone({status:'uploaded'},'remove',true),true);
 assert.equal(selectablePhone({status:'missing'},'upload',true),true);
 for(const status of ['pending','hashing','unknown'])for(const action of ['upload','remove'])assert.equal(selectablePhone({status},action,true),false);
});
test('checking filter includes pending originals and actively hashing originals',()=>{
 assert.equal(filterPhone([{...rows[0],status:'pending'},{...rows[1],status:'hashing'},{...rows[2],status:'uploaded'}],{...f,'status-filter':'pending'}).length,2);
});
