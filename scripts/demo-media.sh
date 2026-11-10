#!/bin/sh
# Downloads the photos and video the demo chats use (tuimessenger demo serve -media DIR):
# photos from Unsplash via picsum.photos (Unsplash License), video: Big Buck Bunny © Blender Foundation (CC BY 3.0).
set -eu
dir=${1:-"$HOME/.cache/tuimessenger-demo-media"}
mkdir -p "$dir"
get() { [ -s "$dir/$1" ] || curl -fsSL "$2" -o "$dir/$1"; }
get food.jpg https://picsum.photos/id/292/1200/800
get mountains.jpg https://picsum.photos/id/1018/1200/800
get desk.jpg https://picsum.photos/id/180/1200/800
get bunny.mp4 https://test-videos.co.uk/vids/bigbuckbunny/mp4/h264/360/Big_Buck_Bunny_360_10s_1MB.mp4
ls -l "$dir"
