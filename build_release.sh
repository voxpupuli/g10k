#! /usr/bin/env bash
#set -e

if [ $# -ne 1 ]; then
  echo "need the version number as argument to create the git tag"
  echo "e.g. ${0} 0.4.5"
  echo "Aborting..."
	exit 1
fi

git pull

time go test -v

if [ $? -ne 0 ]; then 
  echo "Tests unsuccessfull"
  echo "Aborting..."
	exit 1
fi

# try to get the project name from the current working directory
projectname=${PWD##*/}

#sed -i "s/${projectname} version [^ ]*/${projectname} version ${1}/" ${projectname}.go
#git add ${projectname}.go
#git commit -m "bump version to v${1}"

echo "creating git tag v${1}"
git tag v${1}
echo "pushing git tag v${1}"
git push -f --tags
git push

export CGO_ENABLED=0 
export BUILDTIME=$(date -u '+%Y-%m-%d_%H:%M:%S') 
export BUILDVERSION=$(git describe --tags)

### macOS Intel

echo "building and uploading ${projectname}-darwin-amd64"
env GOOS=darwin GOARCH=amd64 go build -ldflags "-X main.buildtime=${BUILDTIME} -X main.buildversion=${BUILDVERSION}" && date
zip ${projectname}-v${1}-darwin-amd64.zip ${projectname}

### macOS ARM

echo "building and uploading ${projectname}-darwin-arm64"
env GOOS=darwin GOARCH=arm64 go build -ldflags "-X main.buildtime=${BUILDTIME} -X main.buildversion=${BUILDVERSION}" && date
zip ${projectname}-v${1}-darwin-arm64.zip ${projectname}

### LINUX

echo "building and uploading ${projectname}-linux-amd64"
  go build -ldflags "-X main.buildtime=${BUILDTIME} -X main.buildversion=${BUILDVERSION}" && date && env ${projectname}_cachedir=/tmp/${projectname} ./${projectname} -config test.yaml -branch benchmark 2>&1
zip ${projectname}-v${1}-linux-amd64.zip ${projectname}

