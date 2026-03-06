echo "Attention!"
echo "---"
echo "Make sure to have arm-linux-gnueabihf-binutils installed first with: brew install arm-linux-gnueabihf-binutils"
echo "Make sure to have brew install --cask gcc-arm-embedded"
echo "---"
echo 
#GOARM=7
env GOOS=linux GOARCH=arm64  GO111MODULE=on CC=arm-none-eabi-gcc CGO_ENABLED=1 go build -o WMLabreaderARMpi -x
