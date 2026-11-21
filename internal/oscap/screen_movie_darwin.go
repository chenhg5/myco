//go:build darwin && cgo

package oscap

/*
#cgo CFLAGS: -x objective-c -Wno-deprecated-declarations
#cgo LDFLAGS: -framework Foundation -framework AVFoundation -framework CoreGraphics -framework CoreMedia
#import <ScreenCaptureKit/ScreenCaptureKit.h>
#import <AVFoundation/AVFoundation.h>
#import <CoreGraphics/CoreGraphics.h>
#include <stdlib.h>
#include <dlfcn.h>

@interface MycoMovie : NSObject <SCRecordingOutputDelegate, SCStreamDelegate, AVCaptureFileOutputRecordingDelegate>
@property(atomic,retain) SCStream *stream;
@property(atomic,retain) SCRecordingOutput *output;
@property(atomic,retain) AVCaptureSession *legacy;
@property(atomic,retain) AVCaptureMovieFileOutput *movie;
@property(atomic,retain) NSString *failure;
@property(atomic,assign) int state;
@property(atomic,assign) BOOL stopping;
@property(atomic,assign) BOOL terminal;
- (void)stop;
- (void)failed:(NSString*)reason;
@end
@implementation MycoMovie
- (void)failed:(NSString*)reason {
 @synchronized(self){if(self.state==-1)return;self.failure=reason;self.state=-1;self.stopping=YES;}
 if(self.stream){[self.stream stopCaptureWithCompletionHandler:^(NSError *error){self.terminal=YES;}];}
 else {[self.legacy stopRunning];self.terminal=YES;}
}
- (void)stop {
 @synchronized(self){self.stopping=YES;if(self.state!=1)return;}
 if(self.stream){[self.stream stopCaptureWithCompletionHandler:^(NSError *error){if(error)[self failed:error.localizedDescription];}];}
 else if(self.movie.isRecording){[self.movie stopRecording];}
}
- (void)recordingOutputDidStartRecording:(SCRecordingOutput*)output {
 @synchronized(self){self.state=1;}
 if(self.stopping)[self stop];
}
- (void)recordingOutputDidFinishRecording:(SCRecordingOutput*)output {@synchronized(self){self.state=2;self.terminal=YES;}}
- (void)recordingOutput:(SCRecordingOutput*)output didFailWithError:(NSError*)error {[self failed:error.localizedDescription];}
- (void)stream:(SCStream*)stream didStopWithError:(NSError*)error {[self failed:error.localizedDescription];}
- (void)captureOutput:(AVCaptureFileOutput*)output didStartRecordingToOutputFileAtURL:(NSURL*)url fromConnections:(NSArray*)connections {
 @synchronized(self){self.state=1;}
 if(self.stopping)[self stop];
}
- (void)captureOutput:(AVCaptureFileOutput*)output didFinishRecordingToOutputFileAtURL:(NSURL*)url fromConnections:(NSArray*)connections error:(NSError*)error {
 if(error && ![error.userInfo[AVErrorRecordingSuccessfullyFinishedKey] boolValue]){[self failed:error.localizedDescription];}
 else {[self.legacy stopRunning];@synchronized(self){self.state=2;self.terminal=YES;}}
}
- (void)dealloc {self.stream=nil;self.output=nil;self.legacy=nil;self.movie=nil;self.failure=nil;[super dealloc];}
@end
static void *mycoMovieStart(const char *path,int region,int x,int y,int w,int h,int microphone,int clicks){
 MycoMovie *m=[[MycoMovie alloc]init];
 NSString *file=[NSString stringWithUTF8String:path];
 CGRect wanted=region?CGRectMake(x,y,w,h):CGDisplayBounds(CGMainDisplayID());
 if(@available(macOS 15.0,*)){
  // Load the system framework lazily, so older supported Macs can still launch.
  static dispatch_once_t once;dispatch_once(&once,^{dlopen("/System/Library/Frameworks/ScreenCaptureKit.framework/ScreenCaptureKit",RTLD_LAZY|RTLD_LOCAL);});
  if(!NSClassFromString(@"SCRecordingOutput")){[m failed:@"ScreenCaptureKit is unavailable"];return m;}
  [NSClassFromString(@"SCShareableContent") getShareableContentExcludingDesktopWindows:NO onScreenWindowsOnly:YES completionHandler:^(SCShareableContent *content,NSError *error){
   if(error){[m failed:error.localizedDescription];return;}
   if(m.stopping){[m failed:@"Screen recording cancelled before capture started"];return;}
   SCDisplay *chosen=nil;
   for(SCDisplay *d in content.displays){if(CGRectContainsRect(d.frame,wanted)){chosen=d;break;}}
   if(!chosen){[m failed:@"The selected region must fit within one connected display"];return;}
   SCContentFilter *filter=[[NSClassFromString(@"SCContentFilter") alloc]initWithDisplay:chosen excludingWindows:@[]];
   SCStreamConfiguration *cfg=[[NSClassFromString(@"SCStreamConfiguration") alloc]init];
   cfg.sourceRect=CGRectMake(wanted.origin.x-chosen.frame.origin.x,wanted.origin.y-chosen.frame.origin.y,wanted.size.width,wanted.size.height);
   // Encode at screen-point size: predictable bounds, even H.264 dimensions.
   cfg.width=MAX(16,((int)wanted.size.width+1)&~1);cfg.height=MAX(16,((int)wanted.size.height+1)&~1);
   cfg.minimumFrameInterval=CMTimeMake(1,30);cfg.showsCursor=YES;cfg.showMouseClicks=clicks;
   cfg.capturesAudio=NO;cfg.captureMicrophone=microphone;
   m.stream=[[[NSClassFromString(@"SCStream") alloc]initWithFilter:filter configuration:cfg delegate:m]autorelease];
   [filter release];[cfg release];
   SCRecordingOutputConfiguration *out=[[NSClassFromString(@"SCRecordingOutputConfiguration") alloc]init];out.outputURL=[NSURL fileURLWithPath:file];out.videoCodecType=AVVideoCodecTypeH264;out.outputFileType=AVFileTypeQuickTimeMovie;
   m.output=[[[NSClassFromString(@"SCRecordingOutput") alloc]initWithConfiguration:out delegate:m]autorelease];[out release];
   NSError *attach=nil;
   if(![m.stream addRecordingOutput:m.output error:&attach]){[m failed:attach.localizedDescription?:@"Could not prepare the recording file"];return;}
   [m.stream startCaptureWithCompletionHandler:^(NSError *startError){if(startError)[m failed:startError.localizedDescription];}];
  }];
 }else{
  dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED,0),^{
   uint32_t count=0;CGGetActiveDisplayList(0,NULL,&count);CGDirectDisplayID displays[count];CGGetActiveDisplayList(count,displays,&count);
   CGDirectDisplayID display=0;CGRect bounds=CGRectZero;
   for(uint32_t i=0;i<count;i++){CGRect b=CGDisplayBounds(displays[i]);if(CGRectContainsRect(b,wanted)){display=displays[i];bounds=b;break;}}
   if(!display){[m failed:@"The selected region must fit within one connected display"];return;}
   AVCaptureSession *session=[[AVCaptureSession alloc]init];m.legacy=session;[session release];
   AVCaptureScreenInput *input=[[AVCaptureScreenInput alloc]initWithDisplayID:display];
   input.cropRect=CGRectMake(wanted.origin.x-bounds.origin.x,bounds.size.height-(wanted.origin.y-bounds.origin.y)-wanted.size.height,wanted.size.width,wanted.size.height);
   input.minFrameDuration=CMTimeMake(1,30);input.capturesCursor=YES;input.capturesMouseClicks=clicks;
   if(![m.legacy canAddInput:input]){[input release];[m failed:@"Could not capture this screen"];return;}
   [m.legacy addInput:input];[input release];
   if(microphone){NSError *e=nil;AVCaptureDeviceInput *audio=[AVCaptureDeviceInput deviceInputWithDevice:[AVCaptureDevice defaultDeviceWithMediaType:AVMediaTypeAudio] error:&e];if(!audio||![m.legacy canAddInput:audio]){[m failed:@"Microphone access is unavailable"];return;}[m.legacy addInput:audio];}
   m.movie=[[[AVCaptureMovieFileOutput alloc]init]autorelease];
   if(![m.legacy canAddOutput:m.movie]){[m failed:@"Could not prepare the recording file"];return;}
   [m.legacy addOutput:m.movie];[m.legacy startRunning];
   [m.movie startRecordingToOutputFileURL:[NSURL fileURLWithPath:file] recordingDelegate:m];
  });
 }
 return m;
}
static int mycoMovieState(void *p){MycoMovie *m=p;@synchronized(m){return m.state;}}
static char *mycoMovieError(void *p){MycoMovie *m=p;@synchronized(m){return strdup([m.failure?:@"Screen recording failed" UTF8String]);}}
static void mycoMovieStop(void *p){MycoMovie *m=p;[m stop];}
// A timed-out caller must not release a delegate while native setup/flush is
// still in flight. Keep its ownership until the terminal callback arrives.
static void mycoMovieFree(void *p){
 MycoMovie *m=p;
 BOOL terminal;@synchronized(m){terminal=m.terminal;}
 if(terminal){[m.legacy stopRunning];[m release];return;}
 dispatch_after(dispatch_time(DISPATCH_TIME_NOW,100*NSEC_PER_MSEC),dispatch_get_global_queue(QOS_CLASS_UTILITY,0),^{mycoMovieFree(m);});
}
*/
import "C"

import (
	"context"
	"time"
	"unsafe"
)

func recordNativeMovie(ctx context.Context, a screenRecordArgs, path string, emit Emit) error {
	p := C.CString(path)
	defer C.free(unsafe.Pointer(p))
	var region, x, y, w, h, mic, clicks C.int
	if r := a.Region; r != nil {
		region = 1
		x = C.int(r.X)
		y = C.int(r.Y)
		w = C.int(r.W)
		h = C.int(r.H)
	}
	if a.Microphone {
		mic = 1
	}
	if a.Clicks {
		clicks = 1
	}
	movie := C.mycoMovieStart(p, region, x, y, w, h, mic, clicks)
	return waitScreenMovie(ctx, a.MaxSeconds, func(kind string, data any) {
		if kind == "started" {
			emit(kind, map[string]any{"path": path})
			return
		}
		emit(kind, data)
	}, screenMovieBackend{
		state: func() int { return int(C.mycoMovieState(movie)) },
		failure: func() string {
			message := C.mycoMovieError(movie)
			defer C.free(unsafe.Pointer(message))
			return C.GoString(message)
		},
		stop: func() { C.mycoMovieStop(movie) }, close: func() { C.mycoMovieFree(movie) },
	}, screenMovieTiming{poll: 20 * time.Millisecond, start: 20 * time.Second, finish: 10 * time.Second})
}
