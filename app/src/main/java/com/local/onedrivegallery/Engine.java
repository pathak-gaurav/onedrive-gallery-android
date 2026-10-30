package com.local.onedrivegallery;

import android.content.Context;
import org.json.JSONObject;
import java.io.File;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.security.KeyStore;
import java.security.cert.X509Certificate;
import java.util.Base64;
import javax.net.ssl.TrustManagerFactory;
import javax.net.ssl.X509TrustManager;

final class Engine {
 static { System.loadLibrary("gallery"); }
 private static native byte[] nativeCall(String operation,String body);
 private static native byte[] nativeInitialize(String dir,String cert);
 private static boolean ready;
 static synchronized void init(Context context) throws Exception {
  if(ready)return;
  TrustManagerFactory factory=TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm());factory.init((KeyStore)null);
  StringBuilder pem=new StringBuilder();
  for(var manager:factory.getTrustManagers())if(manager instanceof X509TrustManager tm)for(X509Certificate cert:tm.getAcceptedIssuers())pem.append("-----BEGIN CERTIFICATE-----\n").append(Base64.getMimeEncoder(64,new byte[]{10}).encodeToString(cert.getEncoded())).append("\n-----END CERTIFICATE-----\n");
  File file=new File(context.getFilesDir(),"trusted-roots.pem");Files.write(file.toPath(),pem.toString().getBytes(StandardCharsets.UTF_8));
  String error=new String(nativeInitialize(context.getFilesDir().getAbsolutePath(),file.getAbsolutePath()),StandardCharsets.UTF_8);
  if(!error.isEmpty())throw new Exception(error);ready=true;
 }
 static JSONObject call(String operation,JSONObject body) throws Exception {
  JSONObject result=new JSONObject(new String(nativeCall(operation,body.toString()),StandardCharsets.UTF_8));
  if(result.has("error"))throw new Exception(result.getString("error"));return result;
 }
 static JSONObject call(String operation) throws Exception{return call(operation,new JSONObject());}
 static JSONObject raw(String operation,JSONObject body) throws Exception{return new JSONObject(new String(nativeCall(operation,body.toString()),StandardCharsets.UTF_8));}
}
