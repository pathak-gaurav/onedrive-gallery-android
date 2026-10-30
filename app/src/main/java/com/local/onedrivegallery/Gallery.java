package com.local.onedrivegallery;

import android.Manifest;
import android.content.ContentUris;
import android.content.Context;
import android.content.pm.PackageManager;
import android.database.Cursor;
import android.database.sqlite.SQLiteDatabase;
import android.database.sqlite.SQLiteOpenHelper;
import android.net.Uri;
import android.os.Build;
import android.os.ParcelFileDescriptor;
import android.provider.MediaStore;
import org.json.JSONArray;
import org.json.JSONObject;
import java.util.HashMap;
import java.util.Map;

final class Gallery {
 static final Map<String,JSONObject> originals=new HashMap<>();
 private static SQLiteDatabase cache;
 private static synchronized SQLiteDatabase cache(Context c){
  if(cache==null)cache=new SQLiteOpenHelper(c.getApplicationContext(),"original-hashes.db",null,1){
   public void onCreate(SQLiteDatabase db){db.execSQL("CREATE TABLE hashes (uri TEXT PRIMARY KEY, size INTEGER, modified INTEGER, generation INTEGER, hash TEXT NOT NULL)");}
   public void onUpgrade(SQLiteDatabase db,int oldVersion,int newVersion){}
  }.getWritableDatabase();return cache;
 }
 static boolean full(Context c){return granted(c,Manifest.permission.READ_MEDIA_IMAGES)&&granted(c,Manifest.permission.READ_MEDIA_VIDEO);}
 static boolean granted(Context c,String permission){return c.checkSelfPermission(permission)==PackageManager.PERMISSION_GRANTED;}
 static JSONArray media(Context c)throws Exception {
  String mediaVersion=MediaStore.getVersion(c,MediaStore.VOLUME_EXTERNAL_PRIMARY);android.content.SharedPreferences prefs=c.getSharedPreferences("gallery-cache",Context.MODE_PRIVATE);if(!mediaVersion.equals(prefs.getString("mediaVersion",""))){cache(c).delete("hashes",null,null);prefs.edit().putString("mediaVersion",mediaVersion).apply();}
  JSONArray rows=new JSONArray();synchronized(originals){originals.clear();}
  for(boolean video:new boolean[]{false,true}){
   Uri collection=video?MediaStore.Video.Media.getContentUri(MediaStore.VOLUME_EXTERNAL):MediaStore.Images.Media.getContentUri(MediaStore.VOLUME_EXTERNAL);
   String[] fields={"_id","_display_name","_size","relative_path","date_modified","width","height","duration","datetaken","generation_modified"};
   String[] imageFields={"_id","_display_name","_size","relative_path","date_modified","width","height","datetaken","generation_modified"};
   try(Cursor cursor=c.getContentResolver().query(collection,video?fields:imageFields,"is_pending=0 AND is_trashed=0",null,"date_modified DESC")){
    if(cursor==null)continue;
    while(cursor.moveToNext()){
     long size=cursor.getLong(2);if(size<=0)continue;
     Uri uri=ContentUris.withAppendedId(collection,cursor.getLong(0));
     JSONObject row=new JSONObject().put("id",uri.toString()).put("name",cursor.getString(1)).put("size",size).put("album",cursor.getString(3)).put("modified",cursor.getLong(4)).put("taken",cursor.getLong(video?8:7)).put("width",cursor.getInt(5)).put("height",cursor.getInt(6)).put("type",video?"video":"image").put("duration",video?cursor.getLong(7):0).put("source","On this phone").put("status","pending");
     row.put("generation",cursor.getLong(video?9:8));
     synchronized(originals){originals.put(uri.toString(),row);}rows.put(row);
    }
   }catch(SecurityException denied){/* The UI reports partial access, and only granted collections are enumerated. */}
  }
  return rows;
 }
 static JSONObject known(String id)throws Exception{synchronized(originals){JSONObject row=originals.get(id);if(row==null)throw new Exception("This phone file is no longer in the comparison. Compare again.");return row;}}
 static ParcelFileDescriptor original(Context c,JSONObject row)throws Exception{
  Uri uri=Uri.parse(row.getString("id"));
  if(!Gallery.granted(c,Manifest.permission.ACCESS_MEDIA_LOCATION)&&row.getString("type").equals("image"))throw new Exception("Allow access to original photo metadata to compare original bytes accurately");
  return c.getContentResolver().openFileDescriptor(MediaStore.setRequireOriginal(uri),"r");
 }
 static String hash(Context c,JSONObject row)throws Exception{
  try(ParcelFileDescriptor fd=original(c,row)){
   if(fd==null)throw new Exception("The local original is unavailable");
   int nativeFd=ParcelFileDescriptor.dup(fd.getFileDescriptor()).detachFd();
   return Engine.call("hash",new JSONObject().put("fd",nativeFd)).getString("hash");
  }
 }
 static String cachedHash(Context c,JSONObject row)throws Exception{
  unchanged(c,row);SQLiteDatabase db=cache(c);
  try(Cursor cur=db.query("hashes",new String[]{"hash"},"uri=? AND size=? AND modified=? AND generation=?",new String[]{row.getString("id"),String.valueOf(row.getLong("size")),String.valueOf(row.getLong("modified")),String.valueOf(row.getLong("generation"))},null,null,null)){
   if(cur.moveToFirst())return cur.getString(0);
  }
  String hash=hash(c,row);unchanged(c,row);android.content.ContentValues values=new android.content.ContentValues();values.put("uri",row.getString("id"));values.put("size",row.getLong("size"));values.put("modified",row.getLong("modified"));values.put("generation",row.getLong("generation"));values.put("hash",hash);db.insertWithOnConflict("hashes",null,values,SQLiteDatabase.CONFLICT_REPLACE);return hash;
 }
 static void unchanged(Context c,JSONObject row)throws Exception {
  try(Cursor cursor=c.getContentResolver().query(Uri.parse(row.getString("id")),new String[]{"_size","date_modified","generation_modified"},null,null,null)){
   if(cursor==null||!cursor.moveToFirst()||cursor.getLong(0)!=row.getLong("size")||cursor.getLong(1)!=row.getLong("modified")||cursor.getLong(2)!=row.optLong("generation"))throw new Exception("A selected phone file changed. Compare again.");
  }
 }
}
