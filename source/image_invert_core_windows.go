package main

// invertClipboardImageRGB returns an independent image with every RGB channel
// inverted while preserving alpha and the selection mask. Transparent pixels
// stay transparent; only their stored RGB values are inverted.
func invertClipboardImageRGB(img ClipboardImage) ClipboardImage {
	if !validClipboardImage(img) {
		return ClipboardImage{}
	}
	out := ClipboardImage{
		W:    img.W,
		H:    img.H,
		Pix:  append([]byte(nil), img.Pix...),
		Mask: append([]bool(nil), img.Mask...),
	}
	for i := 0; i+3 < len(out.Pix); i += 4 {
		out.Pix[i+0] = 255 - out.Pix[i+0]
		out.Pix[i+1] = 255 - out.Pix[i+1]
		out.Pix[i+2] = 255 - out.Pix[i+2]
	}
	return out
}
