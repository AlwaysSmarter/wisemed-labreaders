cd ../../implementation/cobas-u411
env GOOS=linux GOARCH=arm64 GO111MODULE=on CGO_ENABLED=1 go get
env GOOS=linux GOARCH=arm64 GO111MODULE=on CGO_ENABLED=1 go build -o ../../output/cobas-u411/WMCobasU411Reader

cd ../../output/cobas-u411/

sudo ./WMCobasU411Reader