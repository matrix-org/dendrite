# Media API Tests

## Implemented

* functional
    * upload
        * normal case
    * download
        * local file
            * existing
            * non-existing
        * remote file
            * existing
    * thumbnail
        * original file formats
            * JPEG
        * local file
            * existing
        * remote file
            * existing
        * cache
            * cold
            * hot
        * pre-generation according to configuration
            * scale
            * crop
        * dynamic generation
            * cold cache
            * larger than original
            * scale

## TODO

* functional
    * upload
        * file too large
        * 0-byte file?
        * invalid filename
        * invalid content-type
    * download
        * invalid origin
        * invalid media id
    * thumbnail
        * original file formats
            * GIF
            * PNG
            * BMP
            * SVG
            * PDF
            * TIFF
            * WEBP
        * local file
            * non-existing
        * remote file
            * non-existing
        * pre-generation according to configuration
            * manual verification + hash check for regressions?
        * dynamic generation
            * hot cache
            * limit on dimensions?
            * 0x0
            * crop
* load
    * 100 parallel requests
        * same file
        * different local files
        * different remote files
        * pre-generated thumbnails
        * non-pre-generated thumbnails
