package com.local.onedrivegallery;

import android.app.Activity;
import android.content.Intent;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.graphics.Color;
import android.graphics.ImageDecoder;
import android.net.Uri;
import android.os.Bundle;
import android.view.Gravity;
import android.view.WindowInsets;
import android.widget.*;
import org.json.JSONObject;
import java.util.concurrent.Executors;

/** Media previews have no JavaScript bridge and cannot access account tokens. */
public final class PreviewActivity extends Activity {
 private LinearLayout layout;
 private VideoView video;
 private Bitmap image;
 private final java.util.concurrent.ExecutorService worker=Executors.newSingleThreadExecutor();
 private TextView status;
 private String webUrl="";
 @Override public void onCreate(Bundle saved){
  super.onCreate(saved);layout=new LinearLayout(this);layout.setOrientation(LinearLayout.VERTICAL);layout.setBackgroundColor(Color.rgb(15,25,22));layout.setPadding(14,12,14,12);setContentView(layout);
  layout.setOnApplyWindowInsetsListener((v,insets)->{android.graphics.Insets edges=insets.getInsets(WindowInsets.Type.systemBars()|WindowInsets.Type.displayCutout());v.setPadding(14+edges.left,12+edges.top,14+edges.right,12+edges.bottom);return WindowInsets.CONSUMED;});layout.requestApplyInsets();
  Button back=new Button(this);back.setText("← Back to results");back.setOnClickListener(v->finish());layout.addView(back);
  status=new TextView(this);status.setTextColor(Color.WHITE);status.setText("Opening preview…");status.setPadding(8,16,8,16);layout.addView(status);
  worker.execute(()->{
   try{
    String source=getIntent().getStringExtra("source"),id=getIntent().getStringExtra("id");JSONObject item;
    if("phone".equals(source)){
     item=Gallery.known(id);String type=item.getString("type");if(type.equals("video")){Uri uri=Uri.parse(item.getString("id"));runOnUiThread(()->play(uri,item.optString("name")));return;}
     Uri uri=Uri.parse(item.getString("id"));Bitmap decoded=ImageDecoder.decodeBitmap(ImageDecoder.createSource(getContentResolver(),uri),(decoder,info,src)->{int w=info.getSize().getWidth(),h=info.getSize().getHeight();double scale=Math.min(1,2048.0/Math.max(w,h));decoder.setTargetSize(Math.max(1,(int)(w*scale)),Math.max(1,(int)(h*scale)));decoder.setAllocator(ImageDecoder.ALLOCATOR_SOFTWARE);});showImage(decoded,item.optString("name"));
    }else{
     Engine.init(this);item=Engine.call("preview",new JSONObject().put("id",id));webUrl=item.optString("webUrl");String type=item.getString("type");
     if(type.equals("video")){Uri uri=Uri.parse(item.getString("url"));runOnUiThread(()->play(uri,item.optString("name")+" · streaming from OneDrive"));}
     else if(type.equals("image")){JSONObject thumb=Engine.call("thumbnail",new JSONObject().put("id",id));byte[] data=java.util.Base64.getDecoder().decode(thumb.getString("data"));Bitmap decoded=BitmapFactory.decodeByteArray(data,0,data.length);if(decoded==null)throw new Exception("No image preview available");showImage(decoded,item.optString("name")+" · OneDrive thumbnail");}
     else runOnUiThread(()->{openOneDrive();finish();});
    }
   }catch(Exception e){runOnUiThread(()->{status.setText("Preview unavailable: "+e.getMessage());addBrowserButton();});}
  });
 }
 void showImage(Bitmap decoded,String name){runOnUiThread(()->{if(isFinishing()||isDestroyed()){decoded.recycle();return;}image=decoded;status.setText(name);ImageView view=new ImageView(this);view.setScaleType(ImageView.ScaleType.FIT_CENTER);view.setImageBitmap(image);layout.addView(view,new LinearLayout.LayoutParams(-1,0,1));addBrowserButton();});}
 void play(Uri uri,String name){if(isFinishing()||isDestroyed())return;status.setText(name);video=new VideoView(this);layout.addView(video,new LinearLayout.LayoutParams(-1,0,1));MediaController controller=new MediaController(this);controller.setAnchorView(video);video.setMediaController(controller);video.setOnPreparedListener(player->{player.setLooping(false);video.start();});video.setOnErrorListener((player,what,extra)->{status.setText("This video cannot be played here. Open its OneDrive preview or use another player.");return true;});video.setVideoURI(uri);addBrowserButton();}
 void addBrowserButton(){if(webUrl.isEmpty())return;Button browser=new Button(this);browser.setText("Open in OneDrive ↗");browser.setOnClickListener(v->openOneDrive());layout.addView(browser);}
 void openOneDrive(){try{Uri uri=Uri.parse(webUrl);String host=uri.getHost();if(!"https".equals(uri.getScheme())||uri.getUserInfo()!=null||host==null||!(host.equals("onedrive.live.com")||host.endsWith(".sharepoint.com")||host.equals("onedrive.com")||host.endsWith(".onedrive.com")||host.equals("1drv.ms")))throw new Exception("Unsupported OneDrive link");startActivity(new Intent(Intent.ACTION_VIEW,uri).addCategory(Intent.CATEGORY_BROWSABLE));}catch(Exception e){Toast.makeText(this,"Could not open the OneDrive preview",Toast.LENGTH_LONG).show();}}
 @Override protected void onPause(){if(video!=null)video.pause();super.onPause();}
 @Override protected void onDestroy(){if(video!=null)video.stopPlayback();if(image!=null)image.recycle();worker.shutdownNow();super.onDestroy();}
}
