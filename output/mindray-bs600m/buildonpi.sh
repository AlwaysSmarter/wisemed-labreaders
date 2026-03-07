cd ../../output/cobas-pro/

rm -f ./WMCobasProReader
cd ../../implementation/cobas-pro
env GOOS=linux GOARCH=arm64 GO111MODULE=on CGO_ENABLED=1 go get
env GOOS=linux GOARCH=arm64 GO111MODULE=on CGO_ENABLED=1 go build -o ../../output/cobas-pro/WMCobasProReader

cd ../../output/cobas-pro/

sudo ./WMCobasProReader