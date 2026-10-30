package com.local.onedrivegallery;

import android.app.PendingIntent;
import android.content.Context;
import android.net.Uri;
import android.os.SystemClock;
import android.provider.MediaStore;
import org.json.JSONArray;
import org.json.JSONObject;
import java.util.ArrayList;
import java.util.UUID;

final class PhoneDeletion {
 private static JSONArray verified;
 private static String token;
 private static long expires;
 static volatile boolean awaiting;
 static synchronized JSONObject prepare(JSONArray ids)throws Exception{
  verified=new JSONArray(ids.toString());token=UUID.randomUUID().toString();expires=SystemClock.elapsedRealtime()+120000;
  return new JSONObject().put("token",token).put("count",ids.length());
 }
 static synchronized PendingIntent confirmation(Context context,String supplied)throws Exception{
  if(awaiting||verified==null||!supplied.equals(token)||SystemClock.elapsedRealtime()>expires)throw new Exception("Verification expired. Recheck the selected originals.");
  ArrayList<Uri> uris=new ArrayList<>();for(int i=0;i<verified.length();i++){JSONObject row=Gallery.known(verified.getString(i));Gallery.unchanged(context,row);uris.add(Uri.parse(row.getString("id")));}
  PendingIntent pending=MediaStore.createDeleteRequest(context.getContentResolver(),uris);verified=null;token=null;awaiting=true;return pending;
 }
 static synchronized void finished(){awaiting=false;verified=null;token=null;}
}
