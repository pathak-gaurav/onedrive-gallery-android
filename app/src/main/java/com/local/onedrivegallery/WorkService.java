package com.local.onedrivegallery;

import android.app.*;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.os.*;
import org.json.JSONArray;
import org.json.JSONObject;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public final class WorkService extends Service {
 static final Object lock=new Object();
 static JSONObject job=new JSONObject(),result=new JSONObject();
 static volatile boolean stopped;
 static boolean reserved;
 private static final ExecutorService worker=Executors.newSingleThreadExecutor();
 private PowerManager.WakeLock wake;
 private long lastNotification;
 static JSONObject snapshot(){synchronized(lock){try{return new JSONObject(job.toString());}catch(Exception e){return new JSONObject();}}}
 static JSONObject output(){synchronized(lock){try{return new JSONObject(result.toString());}catch(Exception e){return new JSONObject();}}}
 static boolean reserve(String operation)throws Exception{synchronized(lock){if(reserved)return false;reserved=true;stopped=false;job=new JSONObject().put("operation",operation).put("status","running").put("message","Starting…").put("done",0).put("total",0);result=new JSONObject();return true;}}
 static void failedStart(){synchronized(lock){reserved=false;try{job.put("status","error").put("message","Android could not start the task. Open the app and try again.");}catch(Exception ignored){}}}
 void progress(String message,int done,int total){synchronized(lock){try{job.put("message",message).put("done",done).put("total",total);}catch(Exception ignored){}}notifyProgress(message);}
 void notifyProgress(String message){if(SystemClock.elapsedRealtime()-lastNotification<1000)return;lastNotification=SystemClock.elapsedRealtime();try{getSystemService(NotificationManager.class).notify(1,notification(message));}catch(Exception ignored){}}
 Notification notification(String message){
  PendingIntent open=PendingIntent.getActivity(this,0,new Intent(this,MainActivity.class),PendingIntent.FLAG_IMMUTABLE|PendingIntent.FLAG_UPDATE_CURRENT);
  return new Notification.Builder(this,"work").setContentTitle("OneDrive Gallery").setContentText(message).setSmallIcon(android.R.drawable.ic_menu_gallery).setContentIntent(open).setOngoing(true).build();
 }
 @Override public int onStartCommand(Intent intent,int flags,int startId){
  getSystemService(NotificationManager.class).createNotificationChannel(new NotificationChannel("work","Scans and uploads",NotificationManager.IMPORTANCE_LOW));
  startForeground(1,notification("Working…"),ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC);
  wake=((PowerManager)getSystemService(POWER_SERVICE)).newWakeLock(PowerManager.PARTIAL_WAKE_LOCK,"OneDriveGallery:work");wake.acquire(6*60*60*1000L);
  worker.execute(()->{
   try{
    if(intent==null)throw new Exception("Interrupted. Start this operation again.");
    Engine.init(this);Engine.call("resetCancel");String operation=intent.getStringExtra("operation");JSONObject body=new JSONObject(intent.getStringExtra("body"));JSONObject value;
    switch(operation){
     case "login":
      Engine.call("login");while(true){if(stopped)throw new Exception("Sign-in stopped");JSONObject s=Engine.call("status").getJSONObject("login");if(s.optString("status").equals("error"))throw new Exception(s.optString("error"));if(s.optString("status").equals("complete"))break;progress("Finish Microsoft sign-in in your browser",0,0);Thread.sleep(1000);}value=new JSONObject().put("signedIn",true);break;
     case "scan":progress("Reading OneDrive file checksums…",0,0);value=Engine.call("scan",body);break;
     case "compare":value=compare(true);break;
     case "verifyPhoneDelete":value=verifyPhoneDelete(body);break;
     case "delete":
      progress("Rechecking and recycling selected copies…",0,0);JSONObject removed=Engine.raw("delete",body);
      synchronized(lock){job.put("deletionDone",true);}
      progress("Refreshing this OneDrive view…",0,0);Engine.call("resetCancel");value=new JSONObject().put("deletion",removed).put("mode",body.getString("mode"));
      try{value.put("scan",Engine.call("scan",body));}catch(Exception e){value.put("refreshError",e.getMessage());}break;
     case "upload":value=upload(body);break;
     default:throw new Exception("Unknown operation");
    }
    synchronized(lock){result=value;job.put("status","complete").put("message","Done");}
   }catch(Exception e){LiveComparison.finish(false);synchronized(lock){try{job.put("status","error").put("message",e.getMessage()==null?"Operation interrupted":e.getMessage());}catch(Exception ignored){}}}
   finally{synchronized(lock){reserved=false;}if(wake!=null&&wake.isHeld())wake.release();stopForeground(STOP_FOREGROUND_REMOVE);stopSelf(startId);}
  });return START_NOT_STICKY;
 }
 JSONObject compare(boolean refresh)throws Exception{
  progress("Reading local photos and videos…",0,0);JSONArray rows=Gallery.media(this);boolean full=Gallery.full(this);LiveComparison.begin(rows,full);
  Engine.call("invalidateIndex");ExecutorService cloudWorker=Executors.newSingleThreadExecutor();java.util.concurrent.Future<JSONObject> cloud=cloudWorker.submit(()->Engine.call("scan",new JSONObject().put("mode","all")));long seen=-1;
  try{
   while(true){
    if(stopped){Engine.call("cancel");throw new Exception("Paused. Continue comparison to resume the saved cloud scan.");}
    boolean done=cloud.isDone();JSONObject cloudState=Engine.call("status");long version=cloudState.optLong("indexVersion");
    if(version!=seen||done){
     seen=version;boolean finalPass=done&&cloudState.optBoolean("indexComplete");
     for(int start=0;start<rows.length();start+=200){
      if(stopped)throw new Exception("Paused. Continue comparison to resume.");JSONArray descriptors=new JSONArray();int end=Math.min(rows.length(),start+200);
      for(int i=start;i<end;i++){JSONObject row=rows.getJSONObject(i);descriptors.put(new JSONObject().put("size",row.getLong("size")).put("hash",row.optString("hash")));}
      JSONArray matches=Engine.call("matchBatch",new JSONObject().put("files",descriptors)).getJSONArray("matches");
      for(int i=start;i<end;i++){
       JSONObject row=rows.getJSONObject(i),match=matches.getJSONObject(i-start);row.remove("message");
       try{
        if(match.getString("status").equals("hash")){
         if(row.has("hashError"))throw new Exception(row.getString("hashError"));
         row.put("status","hashing").put("provisional",true);LiveComparison.publish(row);progress("Checking original: "+row.getString("name"),i,rows.length());
         try{row.put("hash",Gallery.cachedHash(this,row));}catch(Exception e){row.put("hashError",e.getMessage());throw e;}
         match=Engine.call("match",new JSONObject().put("size",row.getLong("size")).put("hash",row.getString("hash")));
        }
        row.put("status",match.getString("status")).put("matches",match.optJSONArray("matches")==null?new JSONArray():match.getJSONArray("matches")).put("provisional",!finalPass);
        if(match.has("message"))row.put("message",match.getString("message"));
       }catch(Exception e){row.put("status","unknown").put("message",e.getMessage()).put("provisional",!finalPass);}
       LiveComparison.publish(row);
      }
      JSONObject info=LiveComparison.info();progress(finalPass?"Finishing comparison…":"Comparing while OneDrive metadata arrives…",info.getJSONObject("counts").optInt("uploaded"),rows.length());
     }
     if(done){try{cloud.get();}catch(java.util.concurrent.ExecutionException e){throw new Exception(e.getCause().getMessage());}if(!finalPass)throw new Exception("Cloud refresh did not finish. Results remain provisional.");LiveComparison.finish(true);progress("Comparison complete",rows.length(),rows.length());return LiveComparison.info().put("streamed",true).put("comparedAt",System.currentTimeMillis());}
    }
    Thread.sleep(350);
   }
  }finally{if(!cloud.isDone()){Engine.call("cancel");try{cloud.get(10,java.util.concurrent.TimeUnit.SECONDS);}catch(Exception ignored){}}cloudWorker.shutdown();}
 }
 JSONObject verifyPhoneDelete(JSONObject body)throws Exception{
  JSONArray ids=body.getJSONArray("ids");if(ids.length()==0||ids.length()>100)throw new Exception("Select between 1 and 100 matched phone files per deletion batch.");
  for(int i=0;i<ids.length();i++)if(!Gallery.known(ids.getString(i)).getString("status").equals("uploaded"))throw new Exception("Only matched originals can be removed from the phone.");
  progress("Rechecking the selected cloud originals…",0,ids.length());
  for(int i=0;i<ids.length();i++){
   if(stopped)throw new Exception("Phone deletion verification stopped");JSONObject row=Gallery.known(ids.getString(i));progress("Verifying backed-up original "+(i+1)+" of "+ids.length(),i,ids.length());Gallery.unchanged(this,row);String hash=Gallery.hash(this,row);Gallery.unchanged(this,row);Engine.call("verifyMatch",new JSONObject().put("size",row.getLong("size")).put("hash",hash));
  }
  return PhoneDeletion.prepare(ids);
 }
 JSONObject upload(JSONObject body)throws Exception{
  JSONArray ids=body.getJSONArray("ids");if(ids.length()==0)throw new Exception("Select missing files first");String target=body.getString("destination");
  for(int i=0;i<ids.length();i++){JSONObject row=Gallery.known(ids.getString(i));if(!row.getString("status").equals("missing"))throw new Exception("Only files confirmed missing can be uploaded");}
  progress("Refreshing OneDrive before uploading…",0,ids.length());Engine.call("scan",new JSONObject().put("mode","all"));JSONArray outcomes=new JSONArray();
  for(int i=0;i<ids.length();i++){
   if(stopped)break;JSONObject row=Gallery.known(ids.getString(i));progress("Uploading "+(i+1)+" of "+ids.length()+": "+row.getString("name"),i,ids.length());
   try{
    Gallery.unchanged(this,row);String hash=row.optString("hash");
    try(ParcelFileDescriptor fd=Gallery.original(this,row)){
     if(fd==null)throw new Exception("Original unavailable");int nativeFd=ParcelFileDescriptor.dup(fd.getFileDescriptor()).detachFd();
     String album=body.optBoolean("preserveAlbums",true)?row.optString("album",""):"";JSONObject uploaded=Engine.call("upload",new JSONObject().put("fd",nativeFd).put("size",row.getLong("size")).put("hash",hash).put("name",row.getString("name")).put("conflictBehavior",body.optString("conflictBehavior","fail")).put("destination",target+(album.isEmpty()?"":"/"+album.replaceAll("/+$",""))));
     outcomes.put(uploaded.put("name",row.getString("name")).put("id",row.getString("id")));
    }
   }catch(Exception e){if(stopped)break;String reason=e.getMessage()==null?"Upload failed":e.getMessage();outcomes.put(new JSONObject().put("id",row.getString("id")).put("name",row.optString("name")).put("status","error").put("message",reason));}
  }
  Engine.call("resetCancel");stopped=false;JSONObject value=new JSONObject().put("outcomes",outcomes).put("requested",ids.length()).put("destination",target).put("preserveAlbums",body.optBoolean("preserveAlbums",true));
  try{value.put("comparison",compare(true));}catch(Exception e){value.put("refreshError",e.getMessage());}return value;
 }
 @Override public void onTimeout(int startId,int type){stopped=true;try{Engine.call("cancel");}catch(Exception ignored){}synchronized(lock){reserved=false;try{job.put("status","error").put("message","Android stopped the background task. Refresh before retrying.");}catch(Exception ignored){}}stopForeground(STOP_FOREGROUND_REMOVE);stopSelf();}
 @Override public IBinder onBind(Intent intent){return null;}
}
