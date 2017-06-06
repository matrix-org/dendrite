# Media API

This server is responsible for serving `/media` requests as per:

http://matrix.org/docs/spec/client_server/r0.2.0.html#id43

Thumbnailing uses https://github.com/nfnt/resize by default which is a pure golang image scaling library relying on image codecs from the standard library. It is ISC-licensed.

Alternatively one can use `gb build -tags bimg` to use bimg from https://github.com/h2non/bimg (MIT-licensed) which uses libvips from https://github.com/jcupitt/libvips (LGPL v2.1+ -licensed). libvips is a C library and must be installed/built separately. See the github page for details. Also note that libvips in turn has dependencies with a selection of FOSS licenses. bimg and libvips have significantly better performance than nfnt/resize.
