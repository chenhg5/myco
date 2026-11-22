# Keep annotated JavaScript bridge entry points when page support is enabled.
-keepclassmembers class * { @android.webkit.JavascriptInterface <methods>; }
