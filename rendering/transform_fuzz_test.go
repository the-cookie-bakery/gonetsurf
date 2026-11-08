package rendering

import (
	"gonetsurf/layouting/model"
	"testing"

	"gonetsurf/parsers/css/parser"
)

func FuzzTransformFunction(f *testing.F) {
	for _, seed := range []string{
		"translate(10px, 20%)",
		"translateX(-5px)",
		"scale(2, 0.5)",
		"rotate(45deg)",
		"skew(10deg, -5deg)",
		"matrix(1, 0, 0, 1, 10, 20)",
		"rotate3d(1, 0, 0, 45deg)",
		"matrix3d(1,0,0,0, 0,1,0,0, 0,0,1,0, 0,0,0,1)",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source string) {
		values := parser.ParseComponentValueList(source)
		box := &model.Box{Width: 120, Height: 80}
		for _, value := range values {
			if function, ok := value.(*parser.Function); ok {
				_, _ = transformFunction(function, box)
				_, _ = transformFunctionMatrix4(function, box)
			}
		}
	})
}
