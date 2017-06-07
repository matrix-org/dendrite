* functional
    * upload
        * normal case
        * file too large
        * 0-byte file?
        * invalid filename
        * invalid content-type
    * download
        * invalid origin
        * invalid media id
        * local file
            * existing
            * non-existing
        * remote file
            * existing
            * non-existing
    * thumbnail
        * original file formats
            * JPEG
            * GIF
            * PNG
            * BMP
            * SVG
            * PDF
        * local file
            * existing
            * non-existing
        * remote file
            * existing
            * non-existing
        * cache
            * cold
            * hot
        * pre-generation according to configuration
            * manual verification + hash check for regressions?
        * dynamic generation
            * cold cache
            * hot cache
            * larger than original
            * limit on dimensions?
            * 0x0
            * scale
            * crop
* load
    * 100 parallel requests
        * same file
        * different local files
        * different remote files
        * pre-generated thumbnails
        * non-pre-generated thumbnails
