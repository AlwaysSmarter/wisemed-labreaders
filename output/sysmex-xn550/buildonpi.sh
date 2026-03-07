cd ../../implementation/sysmex-xn550

env GOOS=linux GOARCH=arm64 GO111MODULE=on CGO_ENABLED=1 go get
env GOOS=linux GOARCH=arm64 GO111MODULE=on CGO_ENABLED=1 go build -o ../../output/sysmex-xn550/WMSysmexXN550Reader

cd ../../output/sysmex-xn550/

sudo ./WMSysmexXN550Reader