package russian

import (
	"strings"
)

var FontFiles = [...]string{
	"Arial MT.mxf", "Arial MT (Bold).mxf", "Arial MT (Italic).mxf", "Arial MT (Bold Italic).mxf",
	"Arta.mxf", "Arta (Italic).mxf", "Stoclet ITC TT (Bold).mxf",
	"Courier LD.mxf", "Courier LD (Bold).mxf", "Courier LD (Italic).mxf", "Courier LD (Bold Italic).mxf",
	"CordiaUPC.mxf", "CordiaUPC (Bold).mxf",
	"HG-Gothic.mxf", "HG-Gothic (Bold).mxf",
	"Hy Round Gothic.mxf", "Hy Round Gothic (bold).mxf",
	"MHei B5 (Bold).mxf", "MHei GB (Bold).mxf",
}

func FontSourcePath(name string) string {
	return "Fonts/" + strings.TrimSuffix(name, ".mxf") + ".ttf"
}
