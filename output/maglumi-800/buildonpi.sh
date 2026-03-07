cd ../../implementation/sysmex-ca600

env GOOS=linux GOARCH=arm64 GO111MODULE=on CGO_ENABLED=1 go get
env GOOS=linux GOARCH=arm64 GO111MODULE=on CGO_ENABLED=1 go build -o ../../output/sysmex-ca600/WMSysmexReader

cd ../../output/sysmex-ca600/

sudo ./WMSysmexReader