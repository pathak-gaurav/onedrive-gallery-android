param([switch]$Emulator)
$ErrorActionPreference='Stop'
$projectRoot=$PSScriptRoot
$toolRoot=Join-Path $env:LOCALAPPDATA 'OneDrive Gallery Build'
$sdk=Join-Path $env:LOCALAPPDATA 'Android\Sdk'
$go=Join-Path $toolRoot 'go\bin\go.exe'
$jdk=Get-ChildItem -LiteralPath $toolRoot -Directory -Filter 'jdk-17*' | Select-Object -First 1
$gradle=Get-ChildItem -LiteralPath (Join-Path $env:USERPROFILE '.gradle\wrapper\dists\gradle-8.14.3-all') -Filter gradle.bat -Recurse | Select-Object -First 1
if(!$jdk -or !$gradle -or !(Test-Path -LiteralPath $go)){throw 'JDK 17, Gradle 8.14.3 and Go are required. See README.'}
$env:JAVA_HOME=$jdk.FullName
[IO.File]::WriteAllText((Join-Path $projectRoot 'local.properties'),'sdk.dir='+$sdk.Replace('\','/').Replace(':','\:')+"`n")
function Check([string]$step){if($LASTEXITCODE -ne 0){throw "$step failed ($LASTEXITCODE)"}}
Push-Location (Join-Path $projectRoot 'engine')
try {
 & $go test ./...; Check 'Engine tests'
 $env:GOOS='android';$env:CGO_ENABLED='1';$env:CGO_LDFLAGS='-Wl,-z,max-page-size=16384'
 $arch=if($Emulator){'amd64'}else{'arm64'};$abi=if($Emulator){'x86_64'}else{'arm64-v8a'};$prefix=if($Emulator){'x86_64'}else{'aarch64'}
 $env:GOARCH=$arch
 $env:CC=Join-Path $sdk "ndk\29.0.14206865\toolchains\llvm\prebuilt\windows-x86_64\bin\$prefix-linux-android30-clang.cmd"
 $nativeDir=Join-Path $projectRoot "app\src\main\jniLibs\$abi";New-Item -ItemType Directory -Force $nativeDir | Out-Null
 & $go build -trimpath -buildmode=c-shared -ldflags '-s -w' -o (Join-Path $nativeDir 'libgallery.so') .;Check 'Android native engine'
} finally {Pop-Location;Remove-Item Env:\GOOS,Env:\GOARCH,Env:\CGO_ENABLED,Env:\CGO_LDFLAGS,Env:\CC -ErrorAction SilentlyContinue}
Push-Location $projectRoot
try {
 & python generate_licenses.py;Check 'License collection'
 & node --test test_model.mjs test_gallery.mjs;Check 'Interface model tests'
 if($Emulator){& $gradle.FullName :app:assembleDebug :app:assembleDebugAndroidTest '-PtestAbi=x86_64' --console=plain;Check 'Emulator APKs';return}
 & $gradle.FullName :app:assembleRelease :app:lintRelease --console=plain;Check 'Release build and Android lint'
 $signDir=Join-Path $toolRoot 'signing';New-Item -ItemType Directory -Force $signDir | Out-Null
 $key=Join-Path $signDir 'gallery.jks';$passwordFile=Join-Path $signDir 'password.txt'
 if(!(Test-Path -LiteralPath $key)){
  $env:ODG_SIGN_PASS=[Convert]::ToBase64String([Security.Cryptography.RandomNumberGenerator]::GetBytes(32))
  [IO.File]::WriteAllText($passwordFile,$env:ODG_SIGN_PASS)
  & (Join-Path $env:JAVA_HOME 'bin\keytool.exe') -genkeypair -keystore $key -storepass:env ODG_SIGN_PASS -keypass:env ODG_SIGN_PASS -alias gallery -keyalg RSA -keysize 3072 -validity 36500 -dname 'CN=Local OneDrive Gallery';Check 'Signing key generation'
  Remove-Item Env:\ODG_SIGN_PASS
 }
 $outDir=Join-Path $projectRoot 'output';New-Item -ItemType Directory -Force $outDir | Out-Null
 $apk=Join-Path $outDir 'OneDrive-Gallery.apk'
 $androidTools=Join-Path $sdk 'build-tools\36.0.0'
 & (Join-Path $androidTools 'apksigner.bat') sign --ks $key --ks-key-alias gallery --ks-pass "file:$passwordFile" --out $apk app\build\outputs\apk\release\app-release-unsigned.apk;Check 'APK signing'
 & (Join-Path $androidTools 'apksigner.bat') verify --verbose $apk;Check 'APK signature verification'
 & (Join-Path $androidTools 'zipalign.exe') -c -P 16 4 $apk;Check '16 KB APK alignment'
 $digest=(Get-FileHash -LiteralPath $apk -Algorithm SHA256).Hash.ToLower();[IO.File]::WriteAllText((Join-Path $outDir 'OneDrive-Gallery.apk.sha256'),$digest+'  OneDrive-Gallery.apk'+"`n")
 Write-Output "Signed APK: $apk"
} finally {Pop-Location}
