package com.local.onedrivegallery;

import android.Manifest;
import android.app.*;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.graphics.Bitmap;
import android.net.Uri;
import android.os.*;
import android.util.Size;
import android.view.*;
import android.webkit.*;
import android.widget.Toast;
import org.json.JSONObject;
import java.io.*;
import java.util.Base64;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public final class MainActivity extends Activity {
 private WebView web;
 private final ExecutorService requests=Executors.newFixedThreadPool(3);
 private static final String ORIGIN="https://app.local/";
 @Override public void onCreate(Bundle saved){
  super.onCreate(saved);
  android.widget.FrameLayout safeArea=new android.widget.FrameLayout(this);
  safeArea.setBackgroundColor(android.graphics.Color.rgb(248,250,252));
  web=new WebView(this);
  safeArea.addView(web,new android.widget.FrameLayout.LayoutParams(-1,-1));
  safeArea.setOnApplyWindowInsetsListener((v,insets)->{android.graphics.Insets edges=insets.getInsets(WindowInsets.Type.systemBars()|WindowInsets.Type.displayCutout());v.setPadding(edges.left,edges.top,edges.right,edges.bottom);return WindowInsets.CONSUMED;});
  setContentView(safeArea);safeArea.requestApplyInsets();
  WebSettings settings=web.getSettings();settings.setJavaScriptEnabled(true);settings.setDomStorageEnabled(true);settings.setAllowFileAccess(false);settings.setAllowContentAccess(false);settings.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);settings.setSupportMultipleWindows(false);settings.setJavaScriptCanOpenWindowsAutomatically(false);
  web.setWebViewClient(new WebViewClient(){
   @Override public boolean shouldOverrideUrlLoading(WebView view,WebResourceRequest request){String u=request.getUrl().toString();if(u.startsWith(ORIGIN))return false;return true;}
   @Override public WebResourceResponse shouldInterceptRequest(WebView view,WebResourceRequest request){
    Uri uri=request.getUrl();if(!"https".equals(uri.getScheme())||!"app.local".equals(uri.getHost()))return response("text/plain",new byte[0],403);
    String path=uri.getPath();try{
     if(path!=null&&path.startsWith("/thumb/")){
      String id=uri.getQueryParameter("id");if(id==null)throw new Exception("Missing image");byte[] bytes;
      if(path.equals("/thumb/phone")){
       JSONObject row=Gallery.known(id);Bitmap image=getContentResolver().loadThumbnail(Uri.parse(row.getString("id")),new Size(480,480),null);ByteArrayOutputStream out=new ByteArrayOutputStream();image.compress(Bitmap.CompressFormat.JPEG,85,out);image.recycle();bytes=out.toByteArray();
      }else{Engine.init(MainActivity.this);bytes=Base64.getDecoder().decode(Engine.call("thumbnail",new JSONObject().put("id",id)).getString("data"));}
      return response("image/jpeg",bytes,200);
     }
     String name="/".equals(path)?"index.html":path.substring(1);if(!java.util.Set.of("index.html","app.mjs","model.mjs","gallery-model.mjs","style.css","licenses.html").contains(name))return response("text/plain",new byte[0],404);
     String mime=name.endsWith("html")?"text/html":name.endsWith("css")?"text/css":"text/javascript";
     try(InputStream input=getAssets().open(name)){return response(mime,input.readAllBytes(),200);}
    }catch(Exception e){return response("text/plain",new byte[0],404);}
   }
  });
  web.setWebChromeClient(new WebChromeClient());web.addJavascriptInterface(new Bridge(),"Native");web.loadUrl(ORIGIN);
  getOnBackInvokedDispatcher().registerOnBackInvokedCallback(0,()->{if(web.canGoBack())web.goBack();else web.evaluateJavascript("window.appBack?.()",null);});
 }
 private WebResourceResponse response(String mime,byte[] bytes,int code){
  java.util.Map<String,String> headers=new java.util.HashMap<>();headers.put("Cache-Control","no-store");headers.put("X-Content-Type-Options","nosniff");headers.put("Content-Security-Policy","default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'none'; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'");return new WebResourceResponse(mime,"UTF-8",code,code==200?"OK":"Unavailable",headers,new ByteArrayInputStream(bytes));
 }
 private JSONObject permissionState()throws Exception{return new JSONObject().put("full",Gallery.full(this)).put("originals",Gallery.granted(this,Manifest.permission.ACCESS_MEDIA_LOCATION));}
 private void permissions(){
  java.util.ArrayList<String> permissions=new java.util.ArrayList<>();
  permissions.add(Manifest.permission.READ_MEDIA_IMAGES);permissions.add(Manifest.permission.READ_MEDIA_VIDEO);permissions.add(Manifest.permission.POST_NOTIFICATIONS);
  permissions.add(Manifest.permission.READ_MEDIA_VISUAL_USER_SELECTED);
  permissions.add(Manifest.permission.ACCESS_MEDIA_LOCATION);requestPermissions(permissions.toArray(new String[0]),15);
 }
 private void openBrowser(String address,boolean auth)throws Exception{
  Uri u=Uri.parse(address);String host=u.getHost();
  boolean allowed=auth?"http".equals(u.getScheme())&&("127.0.0.1".equals(host)||"localhost".equals(host))&&u.getPort()==53682:
   "https".equals(u.getScheme())&&host!=null&&(host.equals("onedrive.live.com")||host.equals("1drv.ms")||host.equals("onedrive.com")||host.endsWith(".sharepoint.com")||host.endsWith(".onedrive.com"));
  if(!allowed)throw new Exception("Unsupported browser link");
  runOnUiThread(()->{try{startActivity(new Intent(Intent.ACTION_VIEW,u).addCategory(Intent.CATEGORY_BROWSABLE));}catch(Exception e){Toast.makeText(this,"Install or enable a browser to open this link",Toast.LENGTH_LONG).show();}});
 }
 final class Bridge {
  @JavascriptInterface public void request(String id,String operation,String json){
   if(!id.matches("[0-9]{1,12}"))return;
   requests.execute(()->{
    JSONObject value;
    try{
     Engine.init(MainActivity.this);JSONObject body=new JSONObject(json);
     switch(operation){
      case "status":value=Engine.call("status");value.put("job",WorkService.snapshot()).put("permissions",permissionState()).put("comparison",LiveComparison.info());break;
      case "comparison":value=LiveComparison.updates(body.optLong("since",0));break;
      case "permissions":runOnUiThread(()->permissions());value=new JSONObject().put("ok",true);break;
      case "start":
       String task=body.getString("operation");if(!java.util.Set.of("login","scan","compare","upload","delete","verifyPhoneDelete").contains(task))throw new Exception("Unsupported task");if(PhoneDeletion.awaiting||!WorkService.reserve(task))throw new Exception("Wait for the current task to finish");
       Intent intent=new Intent(MainActivity.this,WorkService.class).putExtra("operation",task).putExtra("body",body.optJSONObject("body")==null?"{}":body.getJSONObject("body").toString());
       try{startForegroundService(intent);}catch(Exception e){WorkService.failedStart();throw e;}value=new JSONObject().put("ok",true);break;
      case "result":value=WorkService.output();break;
      case "confirmPhoneDelete":
       PendingIntent deletion=PhoneDeletion.confirmation(MainActivity.this,body.getString("token"));runOnUiThread(()->{try{startIntentSenderForResult(deletion.getIntentSender(),17,null,0,0,0);}catch(Exception e){PhoneDeletion.finished();Toast.makeText(MainActivity.this,"Android could not open the deletion confirmation",Toast.LENGTH_LONG).show();}});value=new JSONObject().put("ok",true);break;
      case "preview":
       String source=body.getString("source"),fileId=body.getString("id");if(!java.util.Set.of("phone","cloud").contains(source))throw new Exception("Unsupported preview source");if(source.equals("phone"))Gallery.known(fileId);
       runOnUiThread(()->startActivity(new Intent(MainActivity.this,PreviewActivity.class).putExtra("source",source).putExtra("id",fileId)));value=new JSONObject().put("ok",true);break;
      case "cancel":WorkService.stopped=true;Engine.call("cancel");value=new JSONObject().put("ok",true);break;
      case "folders":value=Engine.call("folders",body);break;
      case "open":openBrowser(body.getString("url"),false);value=new JSONObject().put("ok",true);break;
      case "openLogin":
       String auth=Engine.call("status").getJSONObject("login").optString("url");if(auth.isEmpty())throw new Exception("Microsoft sign-in is not ready yet");openBrowser(auth,true);value=new JSONObject().put("ok",true);break;
      default:throw new Exception("Unknown app request");
     }
    }catch(Exception e){value=new JSONObject();try{value.put("error",e.getMessage()==null?"The operation could not finish":e.getMessage());}catch(Exception ignored){}}
    String result=value.toString();runOnUiThread(()->{if(web!=null)web.evaluateJavascript("window.nativeResult("+id+","+JSONObject.quote(result)+")",null);});
   });
  }
 }
 @Override protected void onActivityResult(int request,int result,Intent data){super.onActivityResult(request,result,data);if(request==17){PhoneDeletion.finished();try{if(WorkService.reserve("compare"))startForegroundService(new Intent(this,WorkService.class).putExtra("operation","compare").putExtra("body","{}"));}catch(Exception e){WorkService.failedStart();}if(web!=null)web.evaluateJavascript("window.phoneDeleteResult?.("+(result==RESULT_OK)+")",null);}}
 @Override protected void onResume(){super.onResume();if(web!=null)web.evaluateJavascript("window.appResume?.()",null);}
 @Override protected void onDestroy(){if(web!=null){web.removeJavascriptInterface("Native");web.destroy();web=null;}requests.shutdown();super.onDestroy();}
}
