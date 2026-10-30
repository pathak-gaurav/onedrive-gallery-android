//go:build android

#include <jni.h>
#include <stdlib.h>
#include <string.h>
extern char *GalleryCall(char *, char *);
extern char *GalleryInitialize(char *, char *);
extern void GalleryFree(char *);
static char *utf8(JNIEnv *env,jstring value) {
 jclass cls=(*env)->FindClass(env,"java/lang/String");
 jmethodID method=(*env)->GetMethodID(env,cls,"getBytes","(Ljava/lang/String;)[B");
 jstring charset=(*env)->NewStringUTF(env,"UTF-8");
 jbyteArray bytes=(jbyteArray)(*env)->CallObjectMethod(env,value,method,charset);
 jsize length=(*env)->GetArrayLength(env,bytes);char *copy=malloc(length+1);
 (*env)->GetByteArrayRegion(env,bytes,0,length,(jbyte*)copy);copy[length]=0;
 (*env)->DeleteLocalRef(env,bytes);(*env)->DeleteLocalRef(env,charset);(*env)->DeleteLocalRef(env,cls);return copy;
}
static jbyteArray response(JNIEnv *env,char *text) {
 jsize length=(jsize)strlen(text);jbyteArray bytes=(*env)->NewByteArray(env,length);
 (*env)->SetByteArrayRegion(env,bytes,0,length,(jbyte*)text);GalleryFree(text);return bytes;
}
JNIEXPORT jbyteArray JNICALL Java_com_local_onedrivegallery_Engine_nativeCall(JNIEnv *env,jclass cls,jstring op,jstring body) {
 char *o=utf8(env,op),*b=utf8(env,body);char *r=GalleryCall(o,b);free(o);free(b);return response(env,r);
}
JNIEXPORT jbyteArray JNICALL Java_com_local_onedrivegallery_Engine_nativeInitialize(JNIEnv *env,jclass cls,jstring dir,jstring cert) {
 char *d=utf8(env,dir),*c=utf8(env,cert);char *r=GalleryInitialize(d,c);free(d);free(c);return response(env,r);
}
