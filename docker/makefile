DATE:=${shell date "+%Y%m%d"}
COMMITID:=${shell git rev-parse --short HEAD}
BRANCH:=${shell git symbolic-ref --short -q HEAD|tr / _}
TAG?=${BRANCH}-${COMMITID}-${DATE}

APP=coscli
REPO=ccr.ccs.tencentyun.com/qcloud-ti-platform




images: 
	docker build  -t ${REPO}/${APP}:${TAG} -f Dockerfile ..
push:
	docker push  ${REPO}/${APP}:${TAG}
	
release: images
	docker tag  ${REPO}/${APP}:${TAG}  ${REPO}/${APP}:latest
	docker push ${REPO}/${APP}:${TAG}
	docker push ${REPO}/${APP}:latest

