package alac

import "errors"

var (
	errInvalidCookie      = errors.New("alac: invalid magic cookie")
	errUnsupportedVersion = errors.New("alac: unsupported compatible version")
	errUnsupportedElement = errors.New("alac: unsupported element type (CCE/PCE)")
	errInvalidHeader      = errors.New("alac: invalid frame header")
	errInvalidShift       = errors.New("alac: invalid bytesShifted value")
	errBitstreamOverrun   = errors.New("alac: bitstream overrun")
	errSampleOverrun      = errors.New("alac: sample count exceeds buffer")
	errBitDepth           = errors.New("alac: unsupported bit depth")
	errNoALACTrack        = errors.New("alac: no ALAC track found in container")
	errNoChunkOffset      = errors.New("alac: no chunk offset box (stco/co64)")
	errInvalidCo64        = errors.New("alac: invalid co64 payload")
	errNoStsc             = errors.New("alac: no stsc box")
	errInvalidStsc        = errors.New("alac: invalid stsc payload")
	errNoStsz             = errors.New("alac: no stsz box")
	errInvalidStsz        = errors.New("alac: invalid stsz payload")
)
