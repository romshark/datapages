// Package images keeps the images of the documents: 1 file per image,
// named after the SHA-256 of its bytes. Documents share equal images,
// and no image is deleted: any document may refer to it.
package images
