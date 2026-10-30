package com.local.onedrivegallery;

import android.app.Instrumentation;
import android.content.*;
import android.graphics.Bitmap;
import android.net.Uri;
import android.os.*;
import android.provider.MediaStore;
import android.webkit.WebView;
import android.widget.FrameLayout;
import org.json.JSONObject;
import java.io.*;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;

public final class SmokeInstrumentation extends Instrumentation {
 @Override public void onCreate(Bundle arguments){super.onCreate(arguments);start();}
 void require(boolean value,String message)throws Exception{if(!value)throw new Exception(message);}
 String js(WebView web,String code)throws Exception{
  CountDownLatch latch=new CountDownLatch(1);AtomicReference<String> result=new AtomicReference<>();runOnMainSync(()->web.evaluateJavascript(code,value->{result.set(value);latch.countDown();}));require(latch.await(10,TimeUnit.SECONDS),"JavaScript response timeout");return result.get();
 }
 @Override public void onStart(){
  Bundle results=new Bundle();Uri media=null;
  try{
   Context context=getTargetContext();Engine.init(context);require(!Engine.call("status").getBoolean("signedIn"),"Test must use an unsigned-in emulator");
   File file=new File(context.getCacheDir(),"quickxor-test");try(FileOutputStream out=new FileOutputStream(file)){out.write(new byte[]{(byte)0x8d,0x14,0x43,0x54});}
   try(ParcelFileDescriptor fd=ParcelFileDescriptor.open(file,ParcelFileDescriptor.MODE_READ_ONLY)){
    String hash=Engine.call("hash",new JSONObject().put("fd",ParcelFileDescriptor.dup(fd.getFileDescriptor()).detachFd())).getString("hash");require(hash.equals("jaDAEKgAAAAAAAAABAAAAAAAAAA="),"JNI original-file checksum failed");
   }file.delete();
   MainActivity activity=(MainActivity)startActivitySync(new Intent(context,MainActivity.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));waitForIdleSync();FrameLayout root=(FrameLayout)((FrameLayout)activity.findViewById(android.R.id.content)).getChildAt(0);WebView web=(WebView)root.getChildAt(0);
   String loaded="";for(int i=0;i<40;i++){loaded=js(web,"document.querySelector('#login')?.textContent");if(loaded.contains("Sign in to OneDrive"))break;Thread.sleep(250);}require(loaded.contains("Sign in to OneDrive"),"Sign-in screen did not load");
   js(web,"window.savedNativeResult=window.nativeResult; window.nativeResult=(id,data)=>{window.testNative=data}; Native.request('999','status','{}')");String bridged="";for(int i=0;i<40;i++){bridged=js(web,"window.testNative");if(bridged.contains("signedIn"))break;Thread.sleep(100);}require(bridged.contains("signedIn"),"WebView native bridge failed");js(web,"window.nativeResult=window.savedNativeResult");
   JSONObject started=Engine.call("login");require(started.getString("status").equals("running"),"Login did not start");String auth="";for(int i=0;i<50;i++){auth=Engine.call("status").getJSONObject("login").optString("url");if(!auth.isEmpty())break;Thread.sleep(200);}require(auth.startsWith("http://127.0.0.1:53682/")||auth.startsWith("http://localhost:53682/"),"Rclone loopback OAuth did not start");Engine.call("cancel");
   results.putString("stream","PASS: Android native library, original-file FD hash, WebView assets, JavaScript bridge, and Rclone browser sign-in startup. No Microsoft account or cloud mutations used.\n");finish(-1,results);
  }catch(Throwable e){results.putString("stream","FAIL: "+e+"\n");finish(0,results);}
 }
}
