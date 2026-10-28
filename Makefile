
name = yth_vpn
build = go build -ldflags "-s -w"
out = ./bin/$(name)

build_windows:
	GOOS=windows GOARCH=amd64 $(build) -o $(out)_windows_amd64.exe
	upx $(out)_windows_amd64.exe
	zip -j $(out)_windows_amd64.zip $(out)_windows_amd64.exe ./res/wintun/bin/amd64/wintun.dll ./使用方法.txt
	GOOS=windows GOARCH=arm64 $(build) -o $(out)_windows_arm64.exe
	upx $(out)_windows_arm64.exe
	zip -j $(out)_windows_arm64.zip $(out)_windows_arm64.exe ./res/wintun/bin/arm64/wintun.dll ./使用方法.txt


build_mac:
	GOOS=darwin GOARCH=amd64 $(build) -o $(out)_darwin_amd64
	zip -j $(out)_darwin_amd64.zip $(out)_darwin_amd64 ./使用方法.txt

	GOOS=darwin GOARCH=arm64 $(build) -o $(out)_darwin_arm64
	zip -j $(out)_darwin_arm64.zip $(out)_darwin_arm64 ./使用方法.txt


build_linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(build) -o $(out)_linux_amd64
	upx $(out)_linux_amd64
	zip -j $(out)_linux_amd64.zip $(out)_linux_amd64 ./使用方法.txt


build_all: build_windows build_mac build_linux

