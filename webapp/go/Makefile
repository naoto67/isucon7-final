GOPATH := ${PWD}
export GOPATH

build:
		cd src/app && go build -v -o app *.go
		mv src/app/app .

ensure:
		cd src/app && dep ensure -vendor-only

update:
		cd src/app && dep ensure

test:
		go test -v app

vet:
		go vet ./src/app/...
