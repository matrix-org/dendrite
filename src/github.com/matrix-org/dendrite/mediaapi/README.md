# Media API

This server is responsible for serving `/media` requests as per:

http://matrix.org/docs/spec/client_server/r0.2.0.html#id43

Thumbnailing uses bimg from https://github.com/h2non/bimg (MIT-licensed) which uses libvips from https://github.com/jcupitt/libvips (LGPL v2.1+ -licensed). libvips is a C library and must be installed/built separately. See the github page for details.
