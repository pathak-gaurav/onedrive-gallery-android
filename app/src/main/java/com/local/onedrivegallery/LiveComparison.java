package com.local.onedrivegallery;

import org.json.JSONArray;
import org.json.JSONObject;
import java.util.LinkedHashMap;

final class LiveComparison {
 private static final LinkedHashMap<String,JSONObject> rows=new LinkedHashMap<>();
 private static long version,id;
 private static boolean complete,fullAccess;
 private static final java.util.Map<String,Integer> counts=new java.util.HashMap<>();
 static synchronized void begin(JSONArray media,boolean full)throws Exception{
  rows.clear();counts.clear();version=0;id=System.currentTimeMillis();complete=false;fullAccess=full;
  for(int i=0;i<media.length();i++)publish(media.getJSONObject(i));
 }
 static synchronized void publish(JSONObject row)throws Exception{
  String key=row.getString("id");JSONObject copy=new JSONObject(row.toString());copy.remove("hash");copy.remove("hashError");JSONObject previous=rows.get(key);
  if(previous!=null){copy.put("version",previous.optLong("version"));if(copy.toString().equals(previous.toString()))return;counts.merge(previous.optString("status","pending"),-1,Integer::sum);}
  copy.put("version",++version);rows.remove(key);rows.put(key,copy);counts.merge(copy.optString("status","pending"),1,Integer::sum);
 }
 static synchronized void finish(boolean done){complete=done;}
 static synchronized JSONObject info()throws Exception{
  JSONObject values=new JSONObject();for(var entry:counts.entrySet())values.put(entry.getKey(),entry.getValue());
  return new JSONObject().put("id",id).put("version",version).put("total",rows.size()).put("complete",complete).put("fullAccess",fullAccess).put("counts",values);
 }
 static synchronized JSONObject updates(long since)throws Exception{
  JSONArray changed=new JSONArray();long cursor=since;boolean more=false;
  for(JSONObject row:rows.values())if(row.getLong("version")>since){if(changed.length()>=300){more=true;break;}changed.put(row);cursor=row.getLong("version");}
  return info().put("rows",changed).put("cursor",cursor).put("hasMore",more);
 }
}
