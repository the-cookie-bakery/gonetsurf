package rendering

import (
	"bytes"
	"testing"

	"gonetsurf/rendering/interfaces"
	"gonetsurf/rendering/software"
)

type parityRenderer struct {
	inner interfaces.Renderer
}

func (renderer *parityRenderer) Name() string { return "parity-test" }

func (renderer *parityRenderer) Render(scene *interfaces.Scene) (interfaces.Surface, error) {
	return renderer.inner.Render(scene)
}

func (renderer *parityRenderer) Close() error { return renderer.inner.Close() }

func TestSceneRendererMatchesDirectSoftware(t *testing.T) {
	source := `<!doctype html><html><head><style>
		html, body { margin:0; padding:0 }
		#box { position:absolute; left:10px; top:10px; width:60px; height:40px;
			background:linear-gradient(90deg, #ff0000, #0000ff);
			border:3px solid #222222; border-radius:8px; box-shadow:4px 4px 6px #444444 }
		#text { position:absolute; left:12px; top:58px; color:#008000; font-size:16px }
	</style></head><body><div id="box"></div><div id="text">parity</div></body></html>`
	document := mustParseTestPage(t, source)
	engine := buildTestStyleEngine(t, document)
	direct, err := RenderDocumentWithOptions(document, engine, 160, 100, Options{Images: DataURLImageProvider{}})
	if err != nil {
		t.Fatal(err)
	}
	document = mustParseTestPage(t, source)
	engine = buildTestStyleEngine(t, document)
	recorded, err := RenderDocumentWithOptions(document, engine, 160, 100, Options{
		Images:   DataURLImageProvider{},
		Renderer: &parityRenderer{inner: software.NewRenderer()},
	})
	if err != nil {
		t.Fatal(err)
	}
	directPixels := asSoftwareSurface(t, direct).Image().Pix
	recordedPixels := asSoftwareSurface(t, recorded).Image().Pix
	if !bytes.Equal(directPixels, recordedPixels) {
		t.Fatal("scene replay pixels differ from direct software rendering")
	}
}
