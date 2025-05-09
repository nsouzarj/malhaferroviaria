package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"io"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/sqweek/dialog"
	"golang.org/x/image/font/basicfont"
)

// --- Logger Customizado ---
var fileLogger *log.Logger

// --- Constantes Globais ---
const (
	pixelsPerMeter       = 0.01
	cameraScrollSpeed    = 5.0
	popupWidth           = 150
	popupOptionHeight    = 20
	popupPadding         = 5
	popupColorSquareSize = 16
	hitThreshold         = 8.0
	railStrokeWidth      = 1.0
	tooltipPadding       = 4
	minZoom              = 0.1
	maxZoom              = 10.0
)

// --- Tipos de Elementos ---
type ElementType int

const (
	ElementoViaReta ElementType = iota
	ElementoCircuitoVia
	ElementoChaveSimples
)

// --- Estrutura Elemento ---
type Elemento struct {
	Tipo         ElementType `json:"tipo"`
	ID           int         `json:"id"`
	X            float64     `json:"x"`
	Y            float64     `json:"y"`
	Comprimento  float64     `json:"comprimento"`
	Largura      float64     `json:"largura"`
	Rotacao      float64     `json:"rotacao"`
	Cor          color.RGBA  `json:"cor"`
	Espessura    float64     `json:"espessura"`
	ModoCheio    bool        `json:"modoCheio,omitempty"`
	Estado       string      `json:"estado,omitempty"`
	OrientacaoTC string      `json:"orientacaoTC,omitempty"`
}

// --- Estrutura PopupOption ---
type PopupOption struct {
	Label  string
	Rect   image.Rectangle
	Color  *color.RGBA
	Action func()
}

// --- Estrutura Game ---
type Game struct {
	elementos           []Elemento
	proximoElementoID   int
	elementoAtualTipo   ElementType
	startX, startY      float64
	drawingVia          bool
	currentColor        color.RGBA
	thickness           float64
	screenWidth         int
	screenHeight        int
	whitePixel          *ebiten.Image
	colorPalette        map[ebiten.Key]color.RGBA
	colorNames          map[ebiten.Key]string
	cameraOffsetX, cameraOffsetY, cameraZoom float64
	backgroundColor     color.RGBA
	showHelp            bool
	viaCheiaDefault     bool
	popupVisible        bool
	popupX, popupY      int
	popupOptions        []PopupOption
	hoveredElementIndex, selectedElementIndex, movingElementIndex int
	movingElementOffsetX, movingElementOffsetY float64
}

// --- Funções de Inicialização e Logger ---
func NewGame() *Game {
	monitorWidth, monitorHeight := ebiten.Monitor().Size()
	if monitorWidth <= 0 || monitorHeight <= 0 {
		monitorWidth, monitorHeight = 1024, 768
	} else {
		monitorWidth, monitorHeight = int(float64(monitorWidth)*0.9), int(float64(monitorHeight)*0.9)
	}
	fmt.Printf("Tamanho: %dx%d | Escala Base: 1 pixel (world unit) = %.0f metros (zoom 1.0x)\n", monitorWidth, monitorHeight, 1.0/pixelsPerMeter)
	logFile, err := os.OpenFile("game.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND|os.O_TRUNC, 0660)
	var logOutput io.Writer
	if err == nil {
		logOutput = io.MultiWriter(os.Stderr, logFile)
		fmt.Println("Log: 'game.log' e console.")
	} else {
		fmt.Fprintf(os.Stderr, "Erro log (%v), usando console.\n", err)
		logOutput = os.Stderr
	}
	fileLogger = log.New(logOutput, "", log.Ltime|log.Lmicroseconds)
	logln("==== Log (v9.17.10 - Tampas Verticais para Via Cheia e Vazada) ====") // Version increment
	log.SetOutput(logOutput)
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	whiteImg := ebiten.NewImage(1, 1)
	whiteImg.Fill(color.White)
	palette := map[ebiten.Key]color.RGBA{
		ebiten.Key1: {R: 255, G: 0, B: 0, A: 255},
		ebiten.Key2: {R: 0, G: 0, B: 255, A: 255},
		ebiten.Key3: {R: 255, G: 255, B: 0, A: 255},
		ebiten.Key4: {R: 0, G: 255, B: 0, A: 255},
		ebiten.Key5: {R: 0, G: 206, B: 209, A: 255},
	}
	names := map[ebiten.Key]string{
		ebiten.Key1: "Vermelho", ebiten.Key2: "Azul",
		ebiten.Key3: "Amarelo", ebiten.Key4: "Verde",
		ebiten.Key5: "Turquesa",
	}
	return &Game{
		elementos:         []Elemento{}, proximoElementoID: 1, elementoAtualTipo: ElementoViaReta,
		currentColor:      palette[ebiten.Key1], thickness: 8.0,
		screenWidth:       monitorWidth, screenHeight: monitorHeight, whitePixel: whiteImg,
		colorPalette:      palette, colorNames: names,
		cameraOffsetX:     0.0, cameraOffsetY: 0.0, cameraZoom: 1.0,
		backgroundColor:   color.RGBA{R: 0, G: 0, B: 0, A: 255}, showHelp: false, viaCheiaDefault: false,
		popupVisible:      false, selectedElementIndex: -1, hoveredElementIndex: -1, movingElementIndex: -1,
	}
}
func logf(format string, v ...interface{}) { if fileLogger != nil { now := time.Now(); dateStr := now.Format("01/02/2006"); fileLogger.Output(2, fmt.Sprintf(dateStr+" "+format, v...)) } }
func logln(v ...interface{}) { if fileLogger != nil { now := time.Now(); dateStr := now.Format("01/02/2006"); fileLogger.Output(2, dateStr+" "+strings.TrimRight(fmt.Sprintln(v...), "\n")) } }

// --- Funções Helper de Câmera e Coordenadas ---
func (g *Game) screenToWorld(screenX, screenY int) (float64, float64) {
	csX := float64(screenX) - float64(g.screenWidth)/2.0; csY := float64(screenY) - float64(g.screenHeight)/2.0
	return (csX / g.cameraZoom) + g.cameraOffsetX, (csY / g.cameraZoom) + g.cameraOffsetY
}
func (g *Game) worldToScreen(worldX, worldY float64) (float32, float32) {
	rwX := worldX - g.cameraOffsetX; rwY := worldY - g.cameraOffsetY
	return float32(rwX*g.cameraZoom + float64(g.screenWidth)/2.0), float32(rwY*g.cameraZoom + float64(g.screenHeight)/2.0)
}
func calculateLengthMeters(x1,y1,x2,y2 float64) float64 {
	dx:=x2-x1; dy:=y2-y1
	worldUnitsLen:=math.Sqrt(dx*dx+dy*dy)
	if pixelsPerMeter<=0 {return worldUnitsLen}
	return worldUnitsLen / pixelsPerMeter
}

// --- Salvar/Carregar Elementos ---
func (g *Game) saveElements() error { savePath, err := dialog.File().Filter("JSON Malha", "json").Title("Salvar Malha").Save(); if err != nil { if err == dialog.ErrCancelled { logln("Salvar cancelado."); return nil }; logf("ERRO diálogo salvar: %v", err); return err }; if len(savePath) == 0 { logln("Salvar cancelado (caminho vazio)."); return nil }; if !strings.HasSuffix(strings.ToLower(savePath), ".json") { savePath += ".json" }; file, err := os.Create(savePath); if err != nil { logf("ERRO criar '%s': %v", savePath, err); return err }; defer file.Close(); encoder := json.NewEncoder(file); encoder.SetIndent("", "  "); if err = encoder.Encode(g.elementos); err != nil { logf("ERRO codificar Elementos JSON '%s': %v", savePath, err); return err }; logf("Salvo: '%s' (%d elementos)", savePath, len(g.elementos)); return nil }
func (g *Game) loadElements() error { loadPath, err := dialog.File().Filter("JSON Malha", "json").Title("Carregar Malha").Load(); if err != nil { if err == dialog.ErrCancelled { logln("Carregar cancelado."); return nil }; logf("ERRO diálogo carregar: %v", err); return err }; if len(loadPath) == 0 { logln("Carregar cancelado (caminho vazio)."); return nil }; file, err := os.Open(loadPath); if err != nil { logf("ERRO abrir '%s': %v", loadPath, err); return err }; defer file.Close(); var loadedElements []Elemento; decoder := json.NewDecoder(file); if err = decoder.Decode(&loadedElements); err != nil { logf("ERRO decodificar Elementos JSON '%s': %v", loadPath, err); return err }; logf("Decodificação JSON OK. %d elementos lidos.", len(loadedElements)); g.elementos = loadedElements; g.proximoElementoID = 0; for _, el := range g.elementos { if el.ID >= g.proximoElementoID { g.proximoElementoID = el.ID + 1 } }; if g.proximoElementoID == 0 { g.proximoElementoID = 1 }; g.cameraOffsetX = 0; g.cameraOffsetY = 0; g.cameraZoom = 1.0; g.popupVisible = false; g.selectedElementIndex = -1; g.movingElementIndex = -1; g.hoveredElementIndex = -1; logf("Malha carregada, ID=%d, câmera resetada: '%s'", g.proximoElementoID, loadPath); return nil }

// --- Hit Testing ---
func pointSegmentDistance(px,py,ax,ay,bx,by float64) float64 { dx, dy := bx-ax, by-ay; lengthSq := dx*dx + dy*dy; if lengthSq == 0 { return math.Sqrt(math.Pow(px-ax, 2) + math.Pow(py-ay, 2)) }; t := ((px-ax)*dx + (py-ay)*dy) / lengthSq; t = math.Max(0, math.Min(1, t)); closestX := ax + t*dx; closestY := ay + t*dy; return math.Sqrt(math.Pow(px-closestX, 2) + math.Pow(py-closestY, 2)) }
func (g *Game) findClosestElement(worldX, worldY float64) int {
	closestIndex := -1
	minDistScreen := hitThreshold

	for i := len(g.elementos) - 1; i >= 0; i-- {
		el := g.elementos[i]
		var distToEdgeWorld float64 = math.MaxFloat64

		switch el.Tipo {
		case ElementoViaReta:
			comprimentoWorldUnits := el.Comprimento * pixelsPerMeter
			rad := el.Rotacao * math.Pi / 180.0
			endX := el.X + comprimentoWorldUnits*math.Cos(rad)
			endY := el.Y + comprimentoWorldUnits*math.Sin(rad)
			distToCenterlineWorld := pointSegmentDistance(worldX, worldY, el.X, el.Y, endX, endY)
			distToEdgeWorld = distToCenterlineWorld - (el.Espessura / 2.0)
		case ElementoCircuitoVia:
			vertBarLenWorld := el.Largura
			horizStemLenWorld := el.Largura / 2.0
			strokeWidthWorld := el.Espessura
			vBarX1, vBarY1 := el.X, el.Y - vertBarLenWorld / 2.0
			vBarX2, vBarY2 := el.X, el.Y + vertBarLenWorld / 2.0
			distToVertBarCenterlineWorld := pointSegmentDistance(worldX, worldY, vBarX1, vBarY1, vBarX2, vBarY2)
			hStemOriginX, hStemOriginY := el.X, el.Y
			var hStemEndX, hStemEndY float64
			if el.OrientacaoTC == "Invertido" {
				hStemEndX, hStemEndY = el.X - horizStemLenWorld, el.Y
			} else {
				hStemEndX, hStemEndY = el.X + horizStemLenWorld, el.Y
			}
			distToHorizStemCenterlineWorld := pointSegmentDistance(worldX, worldY, hStemOriginX, hStemOriginY, hStemEndX, hStemEndY)
			minDistToCenterlineWorld := math.Min(distToVertBarCenterlineWorld, distToHorizStemCenterlineWorld)
			distToEdgeWorld = minDistToCenterlineWorld - (strokeWidthWorld / 2.0)
		case ElementoChaveSimples:
			raioWorld := el.Espessura
			distToCenterWorld := math.Sqrt(math.Pow(worldX-el.X, 2) + math.Pow(worldY-el.Y, 2))
			distToEdgeWorld = distToCenterWorld - raioWorld
		}
		distToEdgeScreen := distToEdgeWorld * g.cameraZoom
		if distToEdgeScreen < minDistScreen {
			minDistScreen = distToEdgeScreen
			closestIndex = i
		}
	}
	return closestIndex
}

// --- Update ---
func (g *Game) Update() error { if inpututil.IsKeyJustPressed(ebiten.KeyF1) { g.showHelp = !g.showHelp }; if g.showHelp && inpututil.IsKeyJustPressed(ebiten.KeyEscape) { g.showHelp = false; return nil }; popupClicked := false; if g.popupVisible { cursorX, cursorY := ebiten.CursorPosition(); clickPoint := image.Pt(cursorX, cursorY); popupDrawX, popupDrawY := g.calculatePopupDrawPosition(); if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) { clickedOnOption := false; for _, option := range g.popupOptions { optionDrawRect := option.Rect.Add(image.Pt(popupDrawX-g.popupX, popupDrawY-g.popupY)); if clickPoint.In(optionDrawRect) { option.Action(); g.popupVisible = false; popupClicked = true; clickedOnOption = true; break } }; if !clickedOnOption { g.popupVisible = false; popupClicked = true } }; if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) { g.popupVisible = false; popupClicked = true } }; if !g.showHelp && !popupClicked { cursorX, cursorY := ebiten.CursorPosition(); worldCursorX, worldCursorY := g.screenToWorld(cursorX, cursorY); if g.movingElementIndex == -1 && !g.drawingVia && !g.popupVisible { g.hoveredElementIndex = g.findClosestElement(worldCursorX, worldCursorY) } else { g.hoveredElementIndex = -1 }; _, wheelY := ebiten.Wheel(); if wheelY != 0 { worldMouseXBefore, worldMouseYBefore := g.screenToWorld(cursorX, cursorY); zoomFactor := 1.1; if wheelY < 0 { g.cameraZoom /= zoomFactor } else { g.cameraZoom *= zoomFactor }; g.cameraZoom = math.Max(minZoom, math.Min(g.cameraZoom, maxZoom)); worldMouseXAfter, worldMouseYAfter := g.screenToWorld(cursorX, cursorY); g.cameraOffsetX += (worldMouseXBefore - worldMouseXAfter); g.cameraOffsetY += (worldMouseYBefore - worldMouseYAfter) }; if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) && g.movingElementIndex == -1 { clickedIndex := g.findClosestElement(worldCursorX, worldCursorY); if clickedIndex != -1 { g.selectedElementIndex = clickedIndex; g.popupVisible = true; g.popupX, g.popupY = cursorX, cursorY; g.generatePopupOptions(); g.hoveredElementIndex = -1 } else { g.popupVisible = false } }; if inpututil.IsKeyJustPressed(ebiten.KeyT) { g.elementoAtualTipo = ElementoViaReta; logln("Sel: Via Reta") }; if inpututil.IsKeyJustPressed(ebiten.KeyK) { g.elementoAtualTipo = ElementoChaveSimples; logln("Sel: Chave Simples") }; if inpututil.IsKeyJustPressed(ebiten.KeyI) { g.elementoAtualTipo = ElementoCircuitoVia; logln("Sel: Circuito de Via") }; if inpututil.IsKeyJustPressed(ebiten.KeyV) { g.viaCheiaDefault = !g.viaCheiaDefault; logf("Próxima Via: %s", map[bool]string{true: "Cheia", false: "Vazada"}[g.viaCheiaDefault]) }; for key, clr := range g.colorPalette { if inpututil.IsKeyJustPressed(key) { if g.currentColor != clr { g.currentColor = clr; logf("Cor Padrão: %s", g.colorNames[key]) }; break } }; if inpututil.IsKeyJustPressed(ebiten.KeyF2) { g.backgroundColor = color.RGBA{R: 50, G: 50, B: 50, A: 255}; logln("Fundo: Cinza Escuro") }; if inpututil.IsKeyJustPressed(ebiten.KeyF3) { g.backgroundColor = color.RGBA{R: 100, G: 100, B: 120, A: 255}; logln("Fundo: Cinza Azulado") }; if inpututil.IsKeyJustPressed(ebiten.KeyF4) { g.backgroundColor = color.RGBA{R: 240, G: 240, B: 240, A: 255}; logln("Fundo: Branco Gelo") }; prevThickness := g.thickness; if inpututil.IsKeyJustPressed(ebiten.KeyEqual) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadAdd) { g.thickness = math.Min(50, g.thickness+1.0) }; if inpututil.IsKeyJustPressed(ebiten.KeyMinus) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadSubtract) { g.thickness = math.Max(1, g.thickness-1.0) }; if g.thickness != prevThickness { logf("Espessura ViaReta Padrão (mundo): %.1f", g.thickness) }; if inpututil.IsKeyJustPressed(ebiten.KeyC) { g.elementos = []Elemento{}; g.cameraOffsetX = 0; g.cameraOffsetY = 0; g.cameraZoom = 1.0; g.proximoElementoID = 1; g.popupVisible = false; g.selectedElementIndex = -1; g.movingElementIndex = -1; g.hoveredElementIndex = -1; logln("Malha limpa.") }; if inpututil.IsKeyJustPressed(ebiten.KeyS) { g.saveElements() }; if inpututil.IsKeyJustPressed(ebiten.KeyL) { g.loadElements() }; if inpututil.IsKeyJustPressed(ebiten.KeyEscape) { logln("Saindo."); return ebiten.Termination }; if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) { g.popupVisible = false; clickedExistingElementIndex := g.findClosestElement(worldCursorX, worldCursorY); if clickedExistingElementIndex != -1 { g.movingElementIndex = clickedExistingElementIndex; g.selectedElementIndex = clickedExistingElementIndex; el := g.elementos[g.movingElementIndex]; g.movingElementOffsetX = worldCursorX - el.X; g.movingElementOffsetY = worldCursorY - el.Y; g.drawingVia = false; logf("Movendo ID %d", el.ID) } else { g.selectedElementIndex = -1; g.movingElementIndex = -1; switch g.elementoAtualTipo { case ElementoViaReta: g.startX, g.startY = worldCursorX, worldCursorY; g.drawingVia = true; case ElementoCircuitoVia: novoEl := Elemento{Tipo:ElementoCircuitoVia,ID:g.proximoElementoID,X:worldCursorX,Y:worldCursorY,Largura:30,Cor:g.currentColor,Espessura:3,OrientacaoTC:"Normal"}; g.elementos=append(g.elementos,novoEl); g.proximoElementoID++; logf("Add Circ.Via ID %d (Vert.Bar:%.0f, Stroke:%.0f WU)",novoEl.ID, novoEl.Largura, novoEl.Espessura); case ElementoChaveSimples: novoEl := Elemento{Tipo:ElementoChaveSimples,ID:g.proximoElementoID,X:worldCursorX,Y:worldCursorY,Cor:g.currentColor,Espessura:10}; g.elementos=append(g.elementos,novoEl); g.proximoElementoID++; logf("Add Chave ID %d (R:%.0f WU)",novoEl.ID, novoEl.Espessura) } } }; if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) { if g.movingElementIndex != -1 { el := &g.elementos[g.movingElementIndex]; el.X = worldCursorX - g.movingElementOffsetX; el.Y = worldCursorY - g.movingElementOffsetY; g.selectedElementIndex = g.movingElementIndex; g.hoveredElementIndex = -1 } }; if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) { if g.movingElementIndex != -1 { el := g.elementos[g.movingElementIndex]; logf("ID %d movido (%.0f,%.0f)", el.ID, el.X, el.Y); g.selectedElementIndex = g.movingElementIndex; g.movingElementIndex = -1 } else if g.drawingVia { endWorldX, endWorldY := worldCursorX, worldCursorY; if !math.IsNaN(g.startX) && !math.IsNaN(g.startY) { worldPixelDist := math.Sqrt(math.Pow(endWorldX-g.startX,2)+math.Pow(endWorldY-g.startY,2)); if worldPixelDist*g.cameraZoom > 1.0 { lengthM := calculateLengthMeters(g.startX,g.startY,endWorldX,endWorldY); if !math.IsNaN(lengthM) { dx:=endWorldX-g.startX; dy:=endWorldY-g.startY; rot:=math.Atan2(dy,dx)*180/math.Pi; novoEl:=Elemento{Tipo:ElementoViaReta,ID:g.proximoElementoID,X:g.startX,Y:g.startY,Comprimento:lengthM,Rotacao:rot,Cor:g.currentColor,Espessura:g.thickness,ModoCheio:g.viaCheiaDefault}; g.elementos=append(g.elementos,novoEl); g.proximoElementoID++; logf("Add ViaReta ID %d (%.2fm, E:%.0f WU)", novoEl.ID, novoEl.Comprimento, novoEl.Espessura) } } }; g.drawingVia=false; g.startX=math.NaN(); g.startY=math.NaN(); g.selectedElementIndex=-1 } } }

	currentCamScrollSpeed := cameraScrollSpeed / g.cameraZoom
	if ebiten.IsKeyPressed(ebiten.KeyLeft) {
		g.cameraOffsetX -= currentCamScrollSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) {
		g.cameraOffsetX += currentCamScrollSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyUp) {
		g.cameraOffsetY -= currentCamScrollSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyDown) {
		g.cameraOffsetY += currentCamScrollSpeed
	}

	return nil
}

// generatePopupOptions & calculatePopupDrawPosition
func (g *Game) generatePopupOptions() {
	g.popupOptions = []PopupOption{}
	if g.selectedElementIndex < 0 || g.selectedElementIndex >= len(g.elementos) {
		g.popupVisible = false
		return
	}
	currentPopupY := g.popupY + popupPadding
	colorKeys := []ebiten.Key{ebiten.Key1, ebiten.Key2, ebiten.Key3, ebiten.Key4, ebiten.Key5}
	colorStartX := g.popupX + popupPadding
	squareSpacing := popupColorSquareSize + 5
	for i, key := range colorKeys {
		optColor := g.colorPalette[key]
		optionRect := image.Rect(colorStartX+i*squareSpacing, currentPopupY, colorStartX+i*squareSpacing+popupColorSquareSize, currentPopupY+popupColorSquareSize)
		g.popupOptions = append(g.popupOptions, PopupOption{
			Rect:  optionRect, Color: &optColor,
			Action: func(capturedKey ebiten.Key, capturedColor color.RGBA) func() {
				return func() {
					idxToColor := g.selectedElementIndex
					if idxToColor >= 0 && idxToColor < len(g.elementos) {
						g.elementos[idxToColor].Cor = capturedColor
						logf("Cor ID %d -> %s", g.elementos[idxToColor].ID, g.colorNames[capturedKey])
					}
				}
			}(key, optColor),
		})
	}
	currentPopupY += popupColorSquareSize + popupPadding
	if g.elementos[g.selectedElementIndex].Tipo == ElementoCircuitoVia {
		toggleOrientacaoRect := image.Rect(g.popupX+popupPadding, currentPopupY, g.popupX+popupWidth-popupPadding, currentPopupY+popupOptionHeight)
		currentOrientationDisplay := g.elementos[g.selectedElementIndex].OrientacaoTC
		if currentOrientationDisplay == "" { currentOrientationDisplay = "Normal (ト)" } else
        if currentOrientationDisplay == "Normal" { currentOrientationDisplay = "Normal (ト)"} else
        { currentOrientationDisplay = "Invert. (┤)"}

        labelText := fmt.Sprintf("Inverter (%s)", currentOrientationDisplay)
		g.popupOptions = append(g.popupOptions, PopupOption{
			Label: labelText, Rect:  toggleOrientacaoRect,
			Action: func() {
				idxToToggle := g.selectedElementIndex
				if idxToToggle >= 0 && idxToToggle < len(g.elementos) {
                    selEl := &g.elementos[idxToToggle]
					if selEl.OrientacaoTC == "Normal" || selEl.OrientacaoTC == "" {
						selEl.OrientacaoTC = "Invertido"
					} else {
						selEl.OrientacaoTC = "Normal"
					}
					logf("OrientacaoTC ID %d -> %s", selEl.ID, selEl.OrientacaoTC)
				}
			},
		})
		currentPopupY += popupOptionHeight + popupPadding
	}
	deleteRect := image.Rect(g.popupX+popupPadding, currentPopupY, g.popupX+popupWidth-popupPadding, currentPopupY+popupOptionHeight)
	g.popupOptions = append(g.popupOptions, PopupOption{
		Label: "Apagar", Rect:  deleteRect,
		Action: func() {
			idxToDelete := g.selectedElementIndex
			if idxToDelete >= 0 && idxToDelete < len(g.elementos) {
				elID := g.elementos[idxToDelete].ID; elType := g.elementos[idxToDelete].Tipo
				logf("Apagando ID %d (Tipo: %v)", elID, elType)
				g.elementos = append(g.elementos[:idxToDelete], g.elementos[idxToDelete+1:]...)
				g.selectedElementIndex = -1; g.hoveredElementIndex = -1; g.movingElementIndex = -1
			}
		},
	})
	if len(g.popupOptions) == 0 { g.popupVisible = false }
}

func (g *Game) calculatePopupDrawPosition() (int, int) { popupHeight := 0; if len(g.popupOptions) > 0 { maxY := 0; for _, opt := range g.popupOptions { if opt.Rect.Max.Y > maxY { maxY = opt.Rect.Max.Y } }; popupHeight = (maxY - g.popupY) + popupPadding } else { popupHeight = popupPadding*2 + popupColorSquareSize + popupOptionHeight + popupPadding }; drawPopupX := g.popupX; drawPopupY := g.popupY; if drawPopupX+popupWidth > g.screenWidth { drawPopupX = g.screenWidth - popupWidth }; if drawPopupY+popupHeight > g.screenHeight { drawPopupY = g.screenHeight - popupHeight }; if drawPopupX < 0 { drawPopupX = 0 }; if drawPopupY < 0 { drawPopupY = 0 }; return drawPopupX, drawPopupY }

// --- Texto da Ajuda ---
const helpText = ` = = = AJUDA (Pressione F1 ou ESC para fechar) = = =

SELECAO DE ELEMENTO (Adicao):
 T: Via Reta | I: Circ. Via | K: Chave Simples

ADICIONAR:
 - Via Reta: Clique esquerdo em area vazia, arraste e solte.
             Comprimento em metros, Bitola em Unid. Mundo.
 - Outros: Clique esquerdo em area vazia para posicionar.
   - Circ. Via: Desenha um símbolo ト (ou ┤ se invertido).
                Comprimento da barra vertical e espessura do traço
                em Unid. Mundo. Barra horizontal = 1/2 da vertical.
                (Padrão: Barra Vert. L=30, Traço E=3 Unid. Mundo)
   - Chave Simples: Desenha um circulo.
                    Raio em Unid. Mundo.
                    (Padrão: Raio R=10 Unid. Mundo)

MOVER ELEMENTO:
 - Clique esquerdo sobre um elemento e arraste.

EDITAR/APAGAR ELEMENTOS:
 - Clique Direito sobre um elemento para abrir menu.
   (Mudar cor, Inverter Orientacao ト/┤ para Circ.Via, Apagar)
 - Clique Esquerdo nas opcoes do menu.

NAVEGACAO:
 Setas Cima/Baixo/Esquerda/Direita: Mover Camera (Pan)
 Roda do Mouse: Zoom In/Out (centrado no cursor)

VIA RETA (Proximo a ser adicionado):
 1-5: Mudar Cor Padrao
 +, - (Numpad): Aumentar/Diminuir Bitola Padrao (Unid. Mundo)
 V: Alternar Modo Padra1o (Cheia / Vazada)

COR DE FUNDO: F2: Cinza Escuro | F3: Cinza Azulado | F4: Branco Gelo
ARQUIVO: S: Salvar | L: Carregar | C: Limpar Tudo
SAIR: ESC: Fechar Ajuda / Sair do Programa
`

// --- Funções de Desenho ---
func (g *Game) Draw(screen *ebiten.Image) {
	if screen == nil || g.whitePixel == nil { logln("ERRO CRITICO: screen/whitePixel nil"); return }
	screen.Fill(g.backgroundColor)
	cursorX, cursorY := ebiten.CursorPosition()

	for i, el := range g.elementos {
		var drawColor color.RGBA
		isMoving := (i == g.movingElementIndex); isSelectedPopup := (g.popupVisible && i == g.selectedElementIndex && !isMoving)
		isHovered := (i == g.hoveredElementIndex && !isMoving && !isSelectedPopup && !g.drawingVia && !g.popupVisible)
		if isMoving { drawColor = color.RGBA{R: 255, G: 165, B: 0, A: 255} } else if isSelectedPopup { drawColor = color.RGBA{R: 255, G: 255, B: 255, A: 255}
		} else if isHovered { r, gr, b, a := el.Cor.RGBA(); drawColor = color.RGBA{uint8(math.Min(255, float64(r>>8)+60)), uint8(math.Min(255, float64(gr>>8)+60)), uint8(math.Min(255, float64(b>>8)+60)), uint8(a >> 8)}
		} else { drawColor = el.Cor }
		
		screenDrawSizeElement := float32(el.Espessura * g.cameraZoom) 
		currentRailStrokeWidthOnScreen := float32(railStrokeWidth * g.cameraZoom)
		if currentRailStrokeWidthOnScreen < 0.5 { currentRailStrokeWidthOnScreen = 0.5 }

		switch el.Tipo {
		case ElementoViaReta:
			worldUnitsLength := el.Comprimento * pixelsPerMeter
			rad := el.Rotacao * math.Pi / 180.0
			endWorldX := el.X + worldUnitsLength*math.Cos(rad); endWorldY := el.Y + worldUnitsLength*math.Sin(rad)
			screenX1, screenY1 := g.worldToScreen(el.X, el.Y); screenX2, screenY2 := g.worldToScreen(endWorldX, endWorldY)
			
			screenElGauge := float32(el.Espessura * g.cameraZoom)
			if screenElGauge < 1.0 { screenElGauge = 1.0 } 
			halfScreenGauge := screenElGauge / 2.0
			if halfScreenGauge < 0.5 { halfScreenGauge = 0.5 }

			limitY1_upper := screenY1 - halfScreenGauge
			limitY1_lower := screenY1 + halfScreenGauge
			limitY2_upper := screenY2 - halfScreenGauge
			limitY2_lower := screenY2 + halfScreenGauge

			if el.ModoCheio { 
				vertices := []ebiten.Vertex{
					{DstX: screenX1, DstY: limitY1_upper, SrcX: 0, SrcY: 0},
					{DstX: screenX1, DstY: limitY1_lower, SrcX: 0, SrcY: 0},
					{DstX: screenX2, DstY: limitY2_lower, SrcX: 0, SrcY: 0},
					{DstX: screenX2, DstY: limitY2_upper, SrcX: 0, SrcY: 0},
				}
				r, gVal, b, a := drawColor.RGBA()
				colorR, colorG, colorB, colorA := float32(r)/65535.0, float32(gVal)/65535.0, float32(b)/65535.0, float32(a)/65535.0
				for i := range vertices {
					vertices[i].ColorR = colorR; vertices[i].ColorG = colorG; vertices[i].ColorB = colorB; vertices[i].ColorA = colorA
				}
				indices := []uint16{0, 1, 2, 0, 2, 3} 
				op := &ebiten.DrawTrianglesOptions{AntiAlias: true}
				screen.DrawTriangles(vertices, indices, g.whitePixel, op)

			} else { 
				vector.StrokeLine(screen, screenX1, limitY1_upper, screenX2, limitY2_upper, currentRailStrokeWidthOnScreen, drawColor, true)
				vector.StrokeLine(screen, screenX1, limitY1_lower, screenX2, limitY2_lower, currentRailStrokeWidthOnScreen, drawColor, true)
				vector.StrokeLine(screen, screenX1, limitY1_upper, screenX1, limitY1_lower, currentRailStrokeWidthOnScreen, drawColor, true)
				vector.StrokeLine(screen, screenX2, limitY2_upper, screenX2, limitY2_lower, currentRailStrokeWidthOnScreen, drawColor, true)
			}

		case ElementoCircuitoVia:
			screenX, screenY := g.worldToScreen(el.X, el.Y)
			screenVertBarLen := float32(el.Largura * g.cameraZoom)
			screenHorizStemLen := screenVertBarLen / 2.0
			screenStrokeWidthCV := screenDrawSizeElement 
			if screenStrokeWidthCV < 0.5 { screenStrokeWidthCV = 0.5 }
			
			vBarX1 := screenX; vBarY1 := screenY - screenVertBarLen/2.0
			vBarX2 := screenX; vBarY2 := screenY + screenVertBarLen/2.0
			vector.StrokeLine(screen, vBarX1, vBarY1, vBarX2, vBarY2, screenStrokeWidthCV, drawColor, true)
			
			hStemOriginX := screenX; hStemOriginY := screenY
			var hStemEndX, hStemEndY float32
			if el.OrientacaoTC == "Invertido" {
				hStemEndX = screenX - screenHorizStemLen; hStemEndY = screenY
			} else {
				hStemEndX = screenX + screenHorizStemLen; hStemEndY = screenY
			}
			vector.StrokeLine(screen, hStemOriginX, hStemOriginY, hStemEndX, hStemEndY, screenStrokeWidthCV, drawColor, true)
		case ElementoChaveSimples:
			screenX, screenY := g.worldToScreen(el.X, el.Y)
			screenRaio := screenDrawSizeElement 
			if screenRaio < 1.0 { screenRaio = 1.0 }
			vector.DrawFilledCircle(screen, screenX, screenY, screenRaio, drawColor, true)
		}
	}

	if g.drawingVia && !math.IsNaN(g.startX) && !math.IsNaN(g.startY) {
		startScreenX, startScreenY := g.worldToScreen(g.startX, g.startY)
		endScreenX, endScreenY := float32(cursorX), float32(cursorY)
		
		screenThicknessTemp := float32(g.thickness * g.cameraZoom)
		if screenThicknessTemp < 1.0 { screenThicknessTemp = 1.0 }
		halfScreenGaugeDrawing := screenThicknessTemp / 2.0
		if halfScreenGaugeDrawing < 0.5 { halfScreenGaugeDrawing = 0.5 }

		currentRailStrokeWidthOnScreenTemp := float32(railStrokeWidth * g.cameraZoom)
		if currentRailStrokeWidthOnScreenTemp < 0.5 { currentRailStrokeWidthOnScreenTemp = 0.5 }
		
		limitY1_upper_draw := startScreenY - halfScreenGaugeDrawing
		limitY1_lower_draw := startScreenY + halfScreenGaugeDrawing
		limitY2_upper_draw := endScreenY - halfScreenGaugeDrawing
		limitY2_lower_draw := endScreenY + halfScreenGaugeDrawing

		if g.viaCheiaDefault { 
			vertices := []ebiten.Vertex{
				{DstX: startScreenX, DstY: limitY1_upper_draw, SrcX: 0, SrcY: 0},
				{DstX: startScreenX, DstY: limitY1_lower_draw, SrcX: 0, SrcY: 0},
				{DstX: endScreenX,   DstY: limitY2_lower_draw, SrcX: 0, SrcY: 0},
				{DstX: endScreenX,   DstY: limitY2_upper_draw, SrcX: 0, SrcY: 0},
			}
			r, gVal, b, a := g.currentColor.RGBA()
			colorR, colorG, colorB, colorA := float32(r)/65535.0, float32(gVal)/65535.0, float32(b)/65535.0, float32(a)/65535.0
			for i := range vertices {
				vertices[i].ColorR = colorR; vertices[i].ColorG = colorG; vertices[i].ColorB = colorB; vertices[i].ColorA = colorA
			}
			indices := []uint16{0, 1, 2, 0, 2, 3}
			op := &ebiten.DrawTrianglesOptions{AntiAlias: true}
			screen.DrawTriangles(vertices, indices, g.whitePixel, op)
		} else { 
			vector.StrokeLine(screen, startScreenX, limitY1_upper_draw, endScreenX, limitY2_upper_draw, currentRailStrokeWidthOnScreenTemp, g.currentColor, true)
			vector.StrokeLine(screen, startScreenX, limitY1_lower_draw, endScreenX, limitY2_lower_draw, currentRailStrokeWidthOnScreenTemp, g.currentColor, true)
			vector.StrokeLine(screen, startScreenX, limitY1_upper_draw, startScreenX, limitY1_lower_draw, currentRailStrokeWidthOnScreenTemp, g.currentColor, true)
			vector.StrokeLine(screen, endScreenX,   limitY2_upper_draw, endScreenX,   limitY2_lower_draw, currentRailStrokeWidthOnScreenTemp, g.currentColor, true)
		}
	}

	if g.popupVisible { drawPopupX, drawPopupY := g.calculatePopupDrawPosition(); popupDrawHeight := 0; if len(g.popupOptions) > 0 { maxYRel := 0; for _, opt := range g.popupOptions { relY := opt.Rect.Max.Y - g.popupY; if relY > maxYRel { maxYRel = relY } }; popupDrawHeight = maxYRel + popupPadding }; if popupDrawHeight > 0 { vector.DrawFilledRect(screen, float32(drawPopupX), float32(drawPopupY), float32(popupWidth), float32(popupDrawHeight), color.RGBA{R:50,G:50,B:50,A:220}, false) }; offsetX := drawPopupX - g.popupX; offsetY := drawPopupY - g.popupY; for _, option := range g.popupOptions { optionDrawRect := option.Rect.Add(image.Pt(offsetX, offsetY)); if option.Color != nil { vector.DrawFilledRect(screen, float32(optionDrawRect.Min.X), float32(optionDrawRect.Min.Y), float32(optionDrawRect.Dx()), float32(optionDrawRect.Dy()), *option.Color, false); vector.StrokeRect(screen, float32(optionDrawRect.Min.X), float32(optionDrawRect.Min.Y), float32(optionDrawRect.Dx()), float32(optionDrawRect.Dy()), 1, color.White, false) }; if option.Label != "" { tb := text.BoundString(basicfont.Face7x13, option.Label); tx := optionDrawRect.Min.X + (optionDrawRect.Dx()-tb.Dx())/2; ty := optionDrawRect.Min.Y + (optionDrawRect.Dy()+tb.Dy())/2 - 2; text.Draw(screen, option.Label, basicfont.Face7x13, tx, ty, color.White) } } }

	elementTypeStr := ""
	switch g.elementoAtualTipo {
	case ElementoViaReta: elementTypeStr = "Via Reta[T]"
	case ElementoCircuitoVia: elementTypeStr = "Circ.Via[I]"
	case ElementoChaveSimples: elementTypeStr = "Chave[K]"
	default: elementTypeStr = "Desconhecido"
	}
	viaModeStr:="Vazada"; if g.viaCheiaDefault{viaModeStr="Cheia"}
	metersPerScreenPixel := (1.0/pixelsPerMeter)/g.cameraZoom
	statusText := fmt.Sprintf("Cam:%.0f,%.0f(Z:%.2fx)|Esc:1px=%.1fm|Tipo:%s|Via[V]:%s\nFundo[F2-4]|Scroll[Setas]|+/-:BitolaVR(%.0f WU)|S/L:Arq|C:Limpar|ESC:Sair",g.cameraOffsetX,g.cameraOffsetY,g.cameraZoom,metersPerScreenPixel,elementTypeStr,viaModeStr,g.thickness)
	ebitenutil.DebugPrint(screen,statusText)

	if g.showHelp { vector.DrawFilledRect(screen,0,0,float32(g.screenWidth),float32(g.screenHeight),color.RGBA{R:0,G:0,B:0,A:200},false); text.Draw(screen,helpText,basicfont.Face7x13,20,20,color.White) }
}

// Layout
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	logf("Layout START: oldW=%d, oldH=%d, newW=%d, newH=%d", g.screenWidth, g.screenHeight, outsideWidth, outsideHeight)
	logf("Layout START: oldCamX=%.2f, oldCamY=%.2f", g.cameraOffsetX, g.cameraOffsetY)

	g.screenWidth = outsideWidth
	g.screenHeight = outsideHeight

	logf("Layout END: newW=%d, newH=%d. CamX e CamY NÃO foram alterados: newCamX=%.2f, newCamY=%.2f", g.screenWidth, g.screenHeight, g.cameraOffsetX, g.cameraOffsetY)
	return g.screenWidth, g.screenHeight
}

// drawThickLine (usada para desenhar um retângulo rotacionado que segue o ângulo exato da linha)
// Não é mais usada para ViaReta Cheia neste momento, mas mantida para referência ou outros usos.
func drawThickLine(screen *ebiten.Image, whitePixel *ebiten.Image, x1, y1, x2, y2, screenThickness float32, clr color.Color, id string) {
	if screen == nil || whitePixel == nil { logf("ERRO (%s): screen/whitePixel nil", id); return }
	if screenThickness < 0.5 { screenThickness = 0.5 }
	dx := x2 - x1; dy := y2 - y1
	lengthF64 := math.Sqrt(float64(dx*dx) + float64(dy*dy))
	if lengthF64 == 0 { return }

	angle := math.Atan2(float64(dy), float64(dx))
	cosAngle := float32(math.Cos(angle))
	sinAngle := float32(math.Sin(angle))

	halfThick := screenThickness / 2.0
	
	offsetX := halfThick * sinAngle 
	offsetY := halfThick * cosAngle

	v0x := x1 - offsetX; v0y := y1 + offsetY
	v1x := x1 + offsetX; v1y := y1 - offsetY
	v2x := x2 + offsetX; v2y := y2 - offsetY
	v3x := x2 - offsetX; v3y := y2 + offsetY

	coords := []float32{v0x, v0y, v1x, v1y, v2x, v2y, v3x, v3y}
	for i, val := range coords { if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) { logf("ERRO (%s): Vértice NaN/Inf [%d]", id, i); return } }
	r, gVal, b, a := clr.RGBA(); colorR,colorG,colorB,colorA := float32(r)/65535.0, float32(gVal)/65535.0, float32(b)/65535.0, float32(a)/65535.0
	vertices := []ebiten.Vertex{
		{DstX: v0x, DstY: v0y, SrcX:0,SrcY:0, ColorR:colorR,ColorG:colorG,ColorB:colorB,ColorA:colorA},
		{DstX: v1x, DstY: v1y, SrcX:0,SrcY:0, ColorR:colorR,ColorG:colorG,ColorB:colorB,ColorA:colorA},
		{DstX: v2x, DstY: v2y, SrcX:0,SrcY:0, ColorR:colorR,ColorG:colorG,ColorB:colorB,ColorA:colorA},
		{DstX: v3x, DstY: v3y, SrcX:0,SrcY:0, ColorR:colorR,ColorG:colorG,ColorB:colorB,ColorA:colorA},
	}
	indices := []uint16{0, 1, 2, 0, 2, 3}
	op := &ebiten.DrawTrianglesOptions{AntiAlias: true}; screen.DrawTriangles(vertices, indices, whitePixel, op)
}

// main
func main() {
	gameInstance := NewGame()
	ebiten.SetWindowSize(gameInstance.screenWidth, gameInstance.screenHeight)
	ebiten.SetWindowTitle("Editor de Vias (v9.17.10 - Tampas Verticais para Via Cheia e Vazada)") // Version increment
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	logln("Iniciando loop...")
	if err := ebiten.RunGame(gameInstance); err != nil {
		if err != ebiten.Termination { logf("Erro fatal: %v", err) } else { logln("Jogo terminado.") }
	}
	logln("==== Fim ====")
}