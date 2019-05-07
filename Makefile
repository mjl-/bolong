build:
	go build
	go vet

test:
	go test -coverprofile=cover.out
	go tool cover -html=cover.out -o cover.html
	golint

release:
	env GOOS=linux GOARCH=amd64 ./release.sh
	env GOOS=linux GOARCH=386 ./release.sh
	env GOOS=linux GOARCH=arm ./release.sh
	env GOOS=darwin GOARCH=amd64 ./release.sh
	env GOOS=windows GOARCH=amd64 ./release.sh

fmt:
	go fmt

clean:
	go clean
