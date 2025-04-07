#!/bin/sh

if [ $# -ne 1 ] ;then
	echo "usage: docker-demo.sh <image dir>"
	exit 0
fi 

DIR=$1

IMG=iview:demo
docker buildx build -t $IMG .

docker run --rm -it \
	-h `hostname` \
	-v $DIR:/images \
	-v /tmp/.X11-unix:/tmp/.X11-unix \
	-v $HOME/.Xauthority:/root/.Xauthority \
	-e DISPLAY=$DISPLAY \
	$IMG
