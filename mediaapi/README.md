# Media API

This server is responsible for serving `/media` requests as per:

http://matrix.org/docs/spec/client_server/r0.2.0.html#id43

## Scaling libraries

### nfnt/resize (default)

Thumbnailing uses https://github.com/nfnt/resize by default which is a pure golang image scaling library relying on image codecs from the standard library. It is ISC-licensed.

It is multi-threaded and uses Lanczos3 so produces sharp images. Using Lanczos3 all the way makes it slower than some other approaches like bimg. (~845ms in total for pre-generating 32x32-crop, 96x96-crop, 320x240-scale, 640x480-scale and 800x600-scale from a given JPEG image on a given machine.)

See the sample below for image quality with nfnt/resize:

![](nfnt-96x96-crop.jpg)

### bimg (uses libvips C library)

Alternatively one can use `go build -tags bimg` to use bimg from https://github.com/h2non/bimg (MIT-licensed) which uses libvips from https://github.com/jcupitt/libvips (LGPL v2.1+ -licensed). libvips is a C library and must be installed/built separately. See the github page for details. Also note that libvips in turn has dependencies with a selection of FOSS licenses.

bimg and libvips have significantly better performance than nfnt/resize but produce slightly less-sharp images. bimg uses a box filter for downscaling to within about 200% of the target scale and then uses Lanczos3 for the last bit. This is a much faster approach but comes at the expense of sharpness. (~295ms in total for pre-generating 32x32-crop, 96x96-crop, 320x240-scale, 640x480-scale and 800x600-scale from a given JPEG image on a given machine.)

See the sample below for image quality with bimg:

![](bimg-96x96-crop.jpg)
