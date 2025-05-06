package main

import (
	// Pacotes padrão do Go
	"encoding/json" // Para codificar/decodificar dados em formato JSON (salvar/carregar)
	"fmt"           // Para formatação de strings (usado em logs e UI)
	"image"         // Para manipulação básica de imagens (usado em retângulos do popup)
	"image/color"   // Para definições de cores
	"io"            // Necessário para configurar múltiplos outputs de log (arquivo e console)
	"log"           // Para registrar mensagens (erros, informações)
	"math"          // Para funções matemáticas (Sqrt, Pow, Max, Min, IsNaN, IsInf)
	"os"            // Para interação com o sistema operacional (arquivos)
	"strings"       // Para manipulação de strings (usado em HasSuffix)
	"time"          // Importar pacote time para formatação de data customizada

	// Biblioteca Ebitengine e seus submódulos
	"github.com/hajimehoshi/ebiten/v2"          // Núcleo do Ebitengine
	"github.com/hajimehoshi/ebiten/v2/ebitenutil" // Utilitários (DebugPrint)
	"github.com/hajimehoshi/ebiten/v2/inpututil"  // Utilitários para entrada (teclado, mouse)
	"github.com/hajimehoshi/ebiten/v2/text"      // Para desenhar texto na tela
	"github.com/hajimehoshi/ebiten/v2/vector"    // Para desenho vetorial (retângulos, círculos)

	// Biblioteca externa para diálogos de arquivo nativos
	"github.com/sqweek/dialog"

	// Fonte básica para texto simples
	"golang.org/x/image/font/basicfont"
)

// --- Logger Customizado ---

// fileLogger é uma variável global para nosso logger customizado.
// Ele será configurado para escrever no arquivo de log e/ou console com o formato de data desejado.
var fileLogger *log.Logger

// --- Constantes Globais ---
const (
	// pixelsPerMeter define a escala do mundo: quantos pixels na tela representam 1 metro no mundo.
	// 0.01 significa que 1 pixel = 100 metros. Mudar isso afeta o cálculo de LengthMeters.
	pixelsPerMeter = 0.01

	// cameraScrollSpeed define a velocidade de movimento da câmera (em pixels por frame).
	cameraScrollSpeed = 5.0

	// Dimensões e aparência do menu popup contextual.
	popupWidth           = 100 // Largura fixa do popup.
	popupOptionHeight    = 20  // Altura padrão para opções de texto como "Apagar".
	popupPadding         = 5   // Espaçamento interno nas bordas do popup.
	popupColorSquareSize = 16  // Tamanho dos quadrados de seleção de cor.

	// hitThreshold é a distância máxima (em pixels na tela) para considerar um clique
	// como sendo "em cima" de um trilho para abrir o popup.
	hitThreshold = 8.0

	// nodeRadiusFactor determina o raio dos nós (círculos) nas pontas dos trilhos,
	// como um múltiplo da espessura do trilho (dividido por 2).
	nodeRadiusFactor = 1.5

	// nodeOutlineWidth define a espessura da borda desenhada ao redor dos nós.
	nodeOutlineWidth = 1.0

	// tooltipPadding define a margem interna do balão de dica (tooltip).
	tooltipPadding = 4
)

// --- Estruturas de Dados ---

// Track representa um único segmento de trilho na malha.
type Track struct {
	// Coordenadas X e Y dos pontos inicial e final do trilho no *MUNDO*.
	// Letras maiúsculas são essenciais para exportação (e salvamento em JSON).
	// Tags `json:"..."` definem como os campos serão chamados no arquivo JSON.
	X1 float64 `json:"x1"`
	Y1 float64 `json:"y1"`
	X2 float64 `json:"x2"`
	Y2 float64 `json:"y2"`

	// Cor do trilho.
	Color color.RGBA `json:"color"`

	// Espessura visual do trilho (em pixels na tela).
	Thickness float64 `json:"thickness"`

	// Comprimento calculado do trilho em metros, baseado na escala.
	LengthMeters float64 `json:"lengthMeters"`
}

// PopupOption representa uma opção clicável no menu popup.
type PopupOption struct {
	// Texto exibido (se houver).
	Label string
	// Área retangular clicável, relativa à posição *original* do popup.
	Rect image.Rectangle
	// Ponteiro para a cor (se for uma amostra de cor).
	Color *color.RGBA
	// Função a ser executada quando a opção é clicada.
	Action func()
}

// Game contém todo o estado da aplicação.
type Game struct {
	tracks             []Track      // Slice com todos os trilhos.
	startX, startY     float64      // Coordenadas (MUNDO) de início do desenho atual.
	drawing            bool         // Flag: true se está desenhando um novo trilho.
	currentColor       color.RGBA   // Cor selecionada para novos trilhos.
	thickness          float64      // Espessura selecionada para novos trilhos (pixels).
	screenWidth        int          // Largura atual da janela (pixels).
	screenHeight       int          // Altura atual da janela (pixels).
	whitePixel         *ebiten.Image // Textura 1x1 branca para desenho.
	colorPalette       map[ebiten.Key]color.RGBA // Mapa Tecla -> Cor (para seleção).
	colorNames         map[ebiten.Key]string    // Mapa Tecla -> Nome da Cor.
	cameraOffsetX      float64      // Deslocamento horizontal da câmera (MUNDO).
	cameraOffsetY      float64      // Deslocamento vertical da câmera (MUNDO).
	backgroundColor    color.RGBA   // Cor de fundo atual.
	popupVisible       bool         // Flag: true se o popup está visível.
	popupX, popupY     int          // Coordenadas (TELA) originais onde o popup foi aberto.
	selectedTrackIndex int          // Índice do trilho selecionado para o popup (-1 se nenhum).
	popupOptions       []PopupOption // Opções atuais do popup.
	hoveredTrackIndex  int          // Índice do trilho sob o mouse (-1 se nenhum).
}

// --- Funções de Inicialização e Logger ---

// NewGame inicializa e retorna o estado inicial do jogo.
func NewGame() *Game {
	// Determina tamanho da janela.
	monitorWidth, monitorHeight := ebiten.Monitor().Size()
	if monitorWidth <= 0 || monitorHeight <= 0 {
		monitorWidth, monitorHeight = 1024, 768
	} else {
		monitorWidth, monitorHeight = int(float64(monitorWidth)*0.9), int(float64(monitorHeight)*0.9)
	}
	fmt.Printf("Tamanho: %dx%d | Escala: 1 pixel = %.0f metros\n", monitorWidth, monitorHeight, 1.0/pixelsPerMeter)

	// Configura o logger para arquivo e console.
	logFile, err := os.OpenFile("game.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND|os.O_TRUNC, 0660)
	var logOutput io.Writer
	if err == nil {
		logOutput = io.MultiWriter(os.Stderr, logFile) // Loga em ambos.
		fmt.Println("Log configurado para arquivo 'game.log' e console.")
	} else {
		fmt.Fprintf(os.Stderr, "Erro ao abrir arquivo de log (%v), logando apenas no console.\n", err)
		logOutput = os.Stderr // Loga apenas no console.
	}

	// Cria nosso logger customizado sem data padrão, mas com tempo/microssegundos.
	fileLogger = log.New(logOutput, "", log.Ltime|log.Lmicroseconds)
	logln("==== Log (v8.4 - Final Formatting Fix 2) ====") // Primeira mensagem usando logln.

	// Configura o logger padrão global por segurança (útil para libs externas talvez).
	log.SetOutput(logOutput)
	log.SetFlags(log.Ltime | log.Lmicroseconds) // Sem data padrão.

	// Cria a textura 1x1 branca.
	whiteImg := ebiten.NewImage(1, 1)
	whiteImg.Fill(color.White)

	// Define paleta de cores e nomes.
	palette := map[ebiten.Key]color.RGBA{ebiten.Key1: {255, 0, 0, 255}, ebiten.Key2: {0, 0, 255, 255}, ebiten.Key3: {255, 255, 0, 255}, ebiten.Key4: {0, 255, 0, 255}}
	names := map[ebiten.Key]string{ebiten.Key1: "Vermelho", ebiten.Key2: "Azul", ebiten.Key3: "Amarelo", ebiten.Key4: "Verde"}
	initialColorKey := ebiten.Key1

	// Retorna a struct Game preenchida com os valores iniciais.
	return &Game{
		tracks:             []Track{},
		currentColor:       palette[initialColorKey],
		thickness:          5.0,
		screenWidth:        monitorWidth,
		screenHeight:       monitorHeight,
		whitePixel:         whiteImg,
		colorPalette:       palette,
		colorNames:         names,
		cameraOffsetX:      0.0,
		cameraOffsetY:      0.0, // Inicializa offset Y.
		backgroundColor:    color.RGBA{0, 0, 0, 255},
		popupVisible:       false,
		selectedTrackIndex: -1,
		hoveredTrackIndex:  -1,
		popupOptions:       []PopupOption{},
	}
}

// logf formata e escreve uma mensagem no log com data MM/DD/YYYY.
func logf(format string, v ...interface{}) {
	if fileLogger == nil { return } // Segurança: não faz nada se o logger não foi criado.
	now := time.Now()
	dateStr := now.Format("01/02/2006") // Formato MM/DD/YYYY.
	// Cria a mensagem final prefixando a data.
	finalMessage := fmt.Sprintf(dateStr+" "+format, v...)
	// Usa fileLogger.Output para que as flags Ltime|Lmicroseconds sejam adicionadas.
	// Calldepth 2 aponta para a linha que chamou logf, não a linha dentro de logf.
	fileLogger.Output(2, finalMessage)
}

// logln formata e escreve uma mensagem (com nova linha) no log com data MM/DD/YYYY.
func logln(v ...interface{}) {
	if fileLogger == nil { return }
	now := time.Now()
	dateStr := now.Format("01/02/2006") // Formato MM/DD/YYYY.
	// Usa Sprintln para formatar os argumentos como faria log.Println.
	msgStr := fmt.Sprintln(v...)
	// Monta a mensagem, removendo a nova linha extra de Sprintln.
	finalMessage := dateStr + " " + strings.TrimRight(msgStr, "\n")
	fileLogger.Output(2, finalMessage) // Usa Output para adicionar tempo/microssegundos.
}

// --- Funções Helper ---

// screenToWorld converte coordenadas da Tela para o Mundo.
func (g *Game) screenToWorld(screenX, screenY int) (float64, float64) {
	worldX := float64(screenX) + g.cameraOffsetX
	worldY := float64(screenY) + g.cameraOffsetY // Inclui offset Y
	return worldX, worldY
}

// worldToScreen converte coordenadas do Mundo para a Tela.
func (g *Game) worldToScreen(worldX, worldY float64) (float32, float32) {
	screenX := float32(worldX - g.cameraOffsetX)
	screenY := float32(worldY - g.cameraOffsetY) // Inclui offset Y
	return screenX, screenY
}

// calculateLengthMeters calcula o comprimento em metros.
func calculateLengthMeters(x1, y1, x2, y2 float64) float64 {
	dx := x2 - x1; dy := y2 - y1; pixelLength := math.Sqrt(dx*dx + dy*dy); if pixelsPerMeter <= 0 { return pixelLength }; return pixelLength / pixelsPerMeter }

// saveTracks salva os trilhos em um arquivo JSON escolhido pelo usuário.
func (g *Game) saveTracks() error {
	savePath, err := dialog.File().Filter("JSON Malha", "json").Title("Salvar Malha Ferroviária").Save()
	if err != nil { if err == dialog.ErrCancelled { logln("Salvar cancelado."); return nil }; logf("ERRO diálogo salvar: %v", err); return err }
	if len(savePath) == 0 { logln("Salvar cancelado (caminho vazio)."); return nil }
	if !strings.HasSuffix(strings.ToLower(savePath), ".json") { savePath += ".json" }
	file, err := os.Create(savePath); if err != nil { logf("ERRO criar '%s': %v", savePath, err); return err }; defer file.Close()
	encoder := json.NewEncoder(file); encoder.SetIndent("", "  ")
	err = encoder.Encode(g.tracks)
	if err != nil { logf("ERRO codificar JSON '%s': %v", savePath, err); return err }
	logf("Salvo: '%s' (%d trilhos)", savePath, len(g.tracks))
	return nil
}

// loadTracks carrega os trilhos de um arquivo JSON escolhido pelo usuário.
func (g *Game) loadTracks() error {
	loadPath, err := dialog.File().Filter("JSON Malha", "json").Title("Carregar Malha Ferroviária").Load()
	if err != nil { if err == dialog.ErrCancelled { logln("Carregar cancelado."); return nil }; logf("ERRO diálogo carregar: %v", err); return err }
	if len(loadPath) == 0 { logln("Carregar cancelado (caminho vazio)."); return nil }
	file, err := os.Open(loadPath); if err != nil { logf("ERRO abrir '%s': %v", loadPath, err); return err }; defer file.Close()
	var loadedTracks []Track; decoder := json.NewDecoder(file); err = decoder.Decode(&loadedTracks)
	if err != nil { logf("ERRO decodificar JSON '%s': %v", loadPath, err); return err }
	logf("Decodificação JSON OK. %d trilhos lidos.", len(loadedTracks))
	if len(loadedTracks) > 0 { logf("  -> Coords Mundo 1º trilho carregado: (%.1f, %.1f) -> (%.1f, %.1f)", loadedTracks[0].X1, loadedTracks[0].Y1, loadedTracks[0].X2, loadedTracks[0].Y2) } else { logln("  -> Nenhum trilho carregado.") }
	g.tracks = loadedTracks; g.cameraOffsetX = 0; g.cameraOffsetY = 0
	logf("Malha carregada e câmera resetada (X, Y): '%s' (%d trilhos)", loadPath, len(g.tracks))
	return nil
}

// pointSegmentDistance calcula a distância de um ponto a um segmento de linha.
func pointSegmentDistance(px, py, ax, ay, bx, by float64) float64 { dx, dy := bx-ax, by-ay; lengthSq := dx*dx + dy*dy; if lengthSq == 0 { return math.Sqrt(math.Pow(px-ax, 2) + math.Pow(py-ay, 2)) }; t := ((px-ax)*dx + (py-ay)*dy) / lengthSq; t = math.Max(0, math.Min(1, t)); closestX := ax + t*dx; closestY := ay + t*dy; return math.Sqrt(math.Pow(px-closestX, 2) + math.Pow(py-closestY, 2)) }

// findClosestTrack encontra o trilho mais próximo de um ponto no mundo.
func (g *Game) findClosestTrack(worldX, worldY float64) int { closestIndex := -1; minDist := hitThreshold; for i := len(g.tracks) - 1; i >= 0; i-- { track := g.tracks[i]; dist := pointSegmentDistance(worldX, worldY, track.X1, track.Y1, track.X2, track.Y2); effectiveThreshold := hitThreshold + track.Thickness/2.0; if dist < minDist && dist < effectiveThreshold { minDist = dist; closestIndex = i } }; return closestIndex }

// --- Métodos da Interface ebiten.Game ---

// Update lida com a lógica do jogo a cada tick.
func (g *Game) Update() error {
	cursorX, cursorY := ebiten.CursorPosition()
	worldCursorX, worldCursorY := g.screenToWorld(cursorX, cursorY)
	popupClicked := false

	// Atualiza índice do trilho sob o mouse (para tooltip) se popup não visível.
	if !g.popupVisible { g.hoveredTrackIndex = g.findClosestTrack(worldCursorX, worldCursorY) } else { g.hoveredTrackIndex = -1 }

	// Processa interação com o popup (se visível).
	if g.popupVisible {
		clickPoint := image.Pt(cursorX, cursorY); popupDrawX, popupDrawY := g.calculatePopupDrawPosition()
		// Verifica clique esquerdo nas opções.
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			clickedOnOption := false
			for _, option := range g.popupOptions { optionDrawRect := option.Rect.Add(image.Pt(popupDrawX-g.popupX, popupDrawY-g.popupY)); if clickPoint.In(optionDrawRect) { option.Action(); g.popupVisible = false; g.selectedTrackIndex = -1; popupClicked = true; clickedOnOption = true; break } }
			if !clickedOnOption { g.popupVisible = false; g.selectedTrackIndex = -1; popupClicked = true } // Fecha se clicou fora.
		}
		// Verifica clique direito fora das opções para fechar.
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
			inAnyOption := false
			for _, option := range g.popupOptions { optionDrawRect := option.Rect.Add(image.Pt(popupDrawX-g.popupX, popupDrawY-g.popupY)); if clickPoint.In(optionDrawRect) { inAnyOption = true; break } }
			if !inAnyOption { g.popupVisible = false; g.selectedTrackIndex = -1; popupClicked = true }
		}
	}

	// Processa outras interações se o clique não foi no popup.
	if !popupClicked {
		// Abrir popup com clique direito.
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
			clickedIndex := g.findClosestTrack(worldCursorX, worldCursorY)
			if clickedIndex != -1 { g.selectedTrackIndex = clickedIndex; g.popupVisible = true; g.popupX, g.popupY = cursorX, cursorY; g.generatePopupOptions(); g.hoveredTrackIndex = -1
			} else { g.popupVisible = false; g.selectedTrackIndex = -1 }
		}
		// Lógica de desenho.
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) { g.startX, g.startY = worldCursorX, worldCursorY; g.drawing = true; g.popupVisible = false; g.selectedTrackIndex = -1; g.hoveredTrackIndex = -1 }
		if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) && g.drawing { endWorldX, endWorldY := worldCursorX, worldCursorY; if !math.IsNaN(g.startX) && !math.IsNaN(g.startY) { distPx := math.Sqrt(math.Pow(endWorldX-g.startX, 2) + math.Pow(endWorldY-g.startY, 2)); if distPx > 1.0 { lengthM := calculateLengthMeters(g.startX, g.startY, endWorldX, endWorldY); if !math.IsNaN(lengthM) { g.tracks = append(g.tracks, Track{X1: g.startX, Y1: g.startY, X2: endWorldX, Y2: endWorldY, Color: g.currentColor, Thickness: g.thickness, LengthMeters: lengthM}) } else { logf("WARN: Comprimento NaN.") } } } else { logf("WARN: Finalizar sem start válido.") }; g.drawing = false; g.startX = math.NaN(); g.startY = math.NaN() }
	}

	// Scroll da câmera.
	if ebiten.IsKeyPressed(ebiten.KeyLeft) { g.cameraOffsetX -= cameraScrollSpeed }
	if ebiten.IsKeyPressed(ebiten.KeyRight) { g.cameraOffsetX += cameraScrollSpeed }
	if ebiten.IsKeyPressed(ebiten.KeyUp) { g.cameraOffsetY -= cameraScrollSpeed }
	if ebiten.IsKeyPressed(ebiten.KeyDown) { g.cameraOffsetY += cameraScrollSpeed }

	// Seleção de cor do trilho.
	for key, clr := range g.colorPalette { if inpututil.IsKeyJustPressed(key) { if g.currentColor != clr { g.currentColor = clr; logf("Cor trilho: %s", g.colorNames[key]) }; break } }

	// Seleção de cor de fundo.
	if inpututil.IsKeyJustPressed(ebiten.KeyF1) { g.backgroundColor = color.RGBA{0, 0, 0, 255}; logln("Fundo: Preto") }
	if inpututil.IsKeyJustPressed(ebiten.KeyF2) { g.backgroundColor = color.RGBA{50, 50, 50, 255}; logln("Fundo: Cinza Escuro") }
	if inpututil.IsKeyJustPressed(ebiten.KeyF3) { g.backgroundColor = color.RGBA{100, 100, 120, 255}; logln("Fundo: Cinza Azulado") }
	if inpututil.IsKeyJustPressed(ebiten.KeyF4) { g.backgroundColor = color.RGBA{240, 240, 240, 255}; logln("Fundo: Branco Gelo") }

	// Ajuste de espessura.
	prevThickness := g.thickness; if inpututil.IsKeyJustPressed(ebiten.KeyEqual) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadAdd) { g.thickness += 1.0; if g.thickness > 50 { g.thickness = 50 } }; if inpututil.IsKeyJustPressed(ebiten.KeyMinus) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadSubtract) { g.thickness -= 1.0; if g.thickness < 1 { g.thickness = 1 } }; if g.thickness != prevThickness { logf("Espessura: %.1f px", g.thickness) }

	// Limpar.
	if inpututil.IsKeyJustPressed(ebiten.KeyC) { g.tracks = []Track{}; g.cameraOffsetX = 0; g.cameraOffsetY = 0; g.popupVisible = false; g.selectedTrackIndex = -1; g.hoveredTrackIndex = -1; logln("Malha limpa.") }

	// Salvar/Carregar.
	if inpututil.IsKeyJustPressed(ebiten.KeyS) { g.saveTracks() }
	if inpututil.IsKeyJustPressed(ebiten.KeyL) { g.loadTracks() }

	// Sair.
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) { logln("Saindo."); return ebiten.Termination }

	return nil
}

// generatePopupOptions cria as opções (cor, apagar) para o menu popup.
func (g *Game) generatePopupOptions() {
	g.popupOptions = []PopupOption{} // Limpa as opções anteriores.
	// Validação básica do índice selecionado.
	if g.selectedTrackIndex < 0 || g.selectedTrackIndex >= len(g.tracks) { return }

	// Define a ordem das teclas/cores no popup.
	colorKeys := []ebiten.Key{ebiten.Key1, ebiten.Key2, ebiten.Key3, ebiten.Key4}
	// Calcula a posição inicial para desenhar os quadrados de cor.
	colorRowY := g.popupY + popupPadding
	colorStartX := g.popupX + popupPadding
	squareSpacing := popupColorSquareSize + 5 // Espaço entre os quadrados.

	// Cria uma opção para cada cor.
	for i, key := range colorKeys {
		optColor := g.colorPalette[key] // Pega a cor do mapa.
		optIndex := g.selectedTrackIndex // Captura o índice *atual* para a closure.
		// Define o retângulo relativo desta opção de cor.
		optionRect := image.Rect(colorStartX+i*squareSpacing, colorRowY, colorStartX+i*squareSpacing+popupColorSquareSize, colorRowY+popupColorSquareSize)
		// Adiciona a opção à lista.
		g.popupOptions = append(g.popupOptions, PopupOption{
			Rect:  optionRect, // Área clicável.
			Color: &optColor,  // Cor para desenhar o quadrado.
			Action: func() {   // Função closure executada no clique.
				// Verifica novamente se o índice ainda é válido (caso a lista tenha mudado).
				if optIndex >= 0 && optIndex < len(g.tracks) {
					g.tracks[optIndex].Color = optColor // Aplica a cor ao trilho selecionado.
					logf("Cor do trilho %d -> %s", optIndex, g.colorNames[key])
				} else { logf("WARN: Índice inválido %d ao mudar cor via popup", optIndex) }
			},
		})
	}

	// Cria a opção "Apagar".
	deleteRowY := colorRowY + popupColorSquareSize + popupPadding // Abaixo dos quadrados.
	deleteRect := image.Rect(g.popupX+popupPadding, deleteRowY, g.popupX+popupWidth-popupPadding, deleteRowY+popupOptionHeight)
	optIndex := g.selectedTrackIndex // Captura o índice atual para a closure.
	g.popupOptions = append(g.popupOptions, PopupOption{
		Label: "Apagar", // Texto do botão.
		Rect:  deleteRect,
		Action: func() { // Ação de apagar.
			if optIndex >= 0 && optIndex < len(g.tracks) { // Verifica validade.
				logf("Apagando trilho %d", optIndex)
				// Remove o elemento da slice g.tracks.
				g.tracks = append(g.tracks[:optIndex], g.tracks[optIndex+1:]...)
				g.selectedTrackIndex = -1 // Reseta a seleção após apagar.
			} else { logf("WARN: Índice inválido %d ao apagar via popup", optIndex) }
		},
	})
}

// calculatePopupDrawPosition ajusta a posição de desenho do popup para caber na tela.
func (g *Game) calculatePopupDrawPosition() (int, int) {
	// Calcula altura baseada nas opções geradas.
	popupHeight := 0
	if len(g.popupOptions) > 0 { maxY := 0; for _, opt := range g.popupOptions { if opt.Rect.Max.Y > maxY { maxY = opt.Rect.Max.Y } }; popupHeight = (maxY - g.popupY) + popupPadding } else { popupHeight = popupPadding*2 + popupColorSquareSize + popupOptionHeight + popupPadding }

	// Começa na posição original do clique.
	drawPopupX := g.popupX; drawPopupY := g.popupY
	// Ajusta para não sair das bordas.
	if drawPopupX+popupWidth > g.screenWidth { drawPopupX = g.screenWidth - popupWidth }; if drawPopupY+popupHeight > g.screenHeight { drawPopupY = g.screenHeight - popupHeight }; if drawPopupX < 0 { drawPopupX = 0 }; if drawPopupY < 0 { drawPopupY = 0 }
	return drawPopupX, drawPopupY // Retorna posição ajustada.
}

// drawNode desenha um nó circular com contorno.
func drawNode(screen *ebiten.Image, x, y, radius, strokeWidth float32, fillColor, outlineColor color.Color) {
	if radius <= 0 { return } // Segurança.
	vector.DrawFilledCircle(screen, x, y, radius, fillColor, true) // Desenha preenchimento.
	if strokeWidth > 0 { vector.StrokeCircle(screen, x, y, radius, strokeWidth, outlineColor, true) } // Desenha contorno.
}

// Draw desenha toda a cena na tela a cada frame.
func (g *Game) Draw(screen *ebiten.Image) {
	if screen == nil || g.whitePixel == nil { logln("ERRO CRÍTICO: screen/whitePixel nil"); return }
	screen.Fill(g.backgroundColor) // Limpa com a cor de fundo.

	totalLengthMeters := 0.0
	cursorX, cursorY := ebiten.CursorPosition()

	// 1. Desenhar Trilhos e Nós existentes.
	for i, track := range g.tracks {
		if !math.IsNaN(track.LengthMeters) { totalLengthMeters += track.LengthMeters }
		screenX1, screenY1 := g.worldToScreen(track.X1, track.Y1); screenX2, screenY2 := g.worldToScreen(track.X2, track.Y2)

		// Define as cores (normal ou destaque).
		lineDrawColor := track.Color; nodeFillColor := track.Color; nodeOutlineColor := color.White
		if g.popupVisible && i == g.selectedTrackIndex { lineDrawColor = color.RGBA{255, 255, 255, 255}; nodeFillColor = color.RGBA{255, 255, 255, 255}; nodeOutlineColor = color.Black }

		// Desenha a linha.
		drawThickLine(screen, g.whitePixel, screenX1, screenY1, screenX2, screenY2, float32(track.Thickness), lineDrawColor, "track")
		// Desenha os nós nas pontas.
		nodeRadius := float32(track.Thickness*nodeRadiusFactor / 2.0); if nodeRadius < 2.0 { nodeRadius = 2.0 }
		drawNode(screen, screenX1, screenY1, nodeRadius, nodeOutlineWidth, nodeFillColor, nodeOutlineColor)
		drawNode(screen, screenX2, screenY2, nodeRadius, nodeOutlineWidth, nodeFillColor, nodeOutlineColor)
	}

	// 2. Desenhar Linha em Progresso e Nós temporários (se estiver desenhando).
	currentDrawingLengthMeters := 0.0
	if g.drawing && !math.IsNaN(g.startX) && !math.IsNaN(g.startY) {
		currentWorldX, currentWorldY := g.screenToWorld(cursorX, cursorY); currentDrawingLengthMeters = calculateLengthMeters(g.startX, g.startY, currentWorldX, currentWorldY)
		startScreenX, startScreenY := g.worldToScreen(g.startX, g.startY)
		// Desenha a linha.
		drawThickLine(screen, g.whitePixel, startScreenX, startScreenY, float32(cursorX), float32(cursorY), float32(g.thickness), g.currentColor, "drawing-live")
		// Desenha os nós temporários.
		nodeRadius := float32(g.thickness*nodeRadiusFactor / 2.0); if nodeRadius < 2.0 { nodeRadius = 2.0 }
		nodeFillColor := g.currentColor; nodeOutlineColor := color.White
		drawNode(screen, startScreenX, startScreenY, nodeRadius, nodeOutlineWidth, nodeFillColor, nodeOutlineColor)
		drawNode(screen, float32(cursorX), float32(cursorY), nodeRadius, nodeOutlineWidth, nodeFillColor, nodeOutlineColor)
	}

	// 3. Desenhar Popup (se visível).
	if g.popupVisible {
		drawPopupX, drawPopupY := g.calculatePopupDrawPosition() // Pega posição ajustada.
		// Calcula altura para o fundo.
		popupDrawHeight := 0; if len(g.popupOptions) > 0 { maxYRel := 0; for _, opt := range g.popupOptions { relY := opt.Rect.Max.Y - g.popupY; if relY > maxYRel { maxYRel = relY } }; popupDrawHeight = maxYRel + popupPadding }
		// Desenha fundo.
		if popupDrawHeight > 0 { vector.DrawFilledRect(screen, float32(drawPopupX), float32(drawPopupY), float32(popupWidth), float32(popupDrawHeight), color.RGBA{50, 50, 50, 220}, false) }
		// Calcula deslocamento para desenhar opções.
		offsetX := drawPopupX - g.popupX; offsetY := drawPopupY - g.popupY
		// Desenha cada opção.
		for _, option := range g.popupOptions { optionDrawRect := option.Rect.Add(image.Pt(offsetX, offsetY)); if option.Color != nil { vector.DrawFilledRect(screen, float32(optionDrawRect.Min.X), float32(optionDrawRect.Min.Y), float32(optionDrawRect.Dx()), float32(optionDrawRect.Dy()), *option.Color, false); vector.StrokeRect(screen, float32(optionDrawRect.Min.X), float32(optionDrawRect.Min.Y), float32(optionDrawRect.Dx()), float32(optionDrawRect.Dy()), 1, color.White, false) }; if option.Label != "" { textBounds := text.BoundString(basicfont.Face7x13, option.Label); textX := optionDrawRect.Min.X + (optionDrawRect.Dx()-textBounds.Dx())/2; textY := optionDrawRect.Min.Y + (optionDrawRect.Dy()+textBounds.Dy())/2 - 2; text.Draw(screen, option.Label, basicfont.Face7x13, textX, textY, color.White) } }
	}

	// 4. Desenhar Tooltip (se houver trilho sob o mouse e popup/desenho inativos).
	if g.hoveredTrackIndex != -1 && g.hoveredTrackIndex < len(g.tracks) && !g.popupVisible && !g.drawing {
		hoveredTrack := g.tracks[g.hoveredTrackIndex]; tooltipText := fmt.Sprintf("%.0f m", hoveredTrack.LengthMeters)
		textBounds := text.BoundString(basicfont.Face7x13, tooltipText); tooltipW := textBounds.Dx() + tooltipPadding*2; tooltipH := 13 + tooltipPadding*2
		tooltipX := cursorX + 10; tooltipY := cursorY + 15 // Posição inicial.
		// Ajusta para caber na tela.
		if tooltipX+tooltipW > g.screenWidth { tooltipX = g.screenWidth - tooltipW }; if tooltipY+tooltipH > g.screenHeight { tooltipY = g.screenHeight - tooltipH }; if tooltipX < 0 { tooltipX = 0 }; if tooltipY < 0 { tooltipY = 0 }
		// Desenha fundo e texto.
		vector.DrawFilledRect(screen, float32(tooltipX), float32(tooltipY), float32(tooltipW), float32(tooltipH), color.RGBA{30, 30, 30, 200}, false)
		text.Draw(screen, tooltipText, basicfont.Face7x13, tooltipX+tooltipPadding, tooltipY+tooltipPadding+10, color.White)
	}

	// 5. Info na Tela (DebugPrint por último para ficar por cima).
	trilhoColorHelp := "Trilho [1:R 2:B 3:Y 4:G]"; bgColorHelp := "Fundo [F1-F4]"; scrollHelp := "Scroll [Setas]"; saveLoadHelp := "Salvar [S] | Carregar [L]"
	statusText := fmt.Sprintf("Comp:%.0fm|Cam:%.0f,%.0f|Esc:1px=%.0fm\n%s|%s|+/-:Esp(%.0fpx)\nArrastar|%s|%s|C:Limpar|ESC:Sair", totalLengthMeters, g.cameraOffsetX, g.cameraOffsetY, 1.0/pixelsPerMeter, trilhoColorHelp, bgColorHelp, g.thickness, scrollHelp, saveLoadHelp)
	if g.drawing && currentDrawingLengthMeters > 0 && !math.IsNaN(currentDrawingLengthMeters) { statusText += fmt.Sprintf("|Atual:%.0fm", currentDrawingLengthMeters) }
	ebitenutil.DebugPrint(screen, statusText)
}

// Layout define o tamanho lógico da tela.
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	// Atualiza as dimensões internas para a lógica de UI (popup/tooltip).
	g.screenWidth = outsideWidth
	g.screenHeight = outsideHeight
	return g.screenWidth, g.screenHeight
}

// drawThickLine desenha uma linha grossa como um retângulo feito de triângulos.
// Recebe coordenadas da TELA.
func drawThickLine(screen *ebiten.Image, whitePixel *ebiten.Image, x1, y1, x2, y2, thickness float32, clr color.Color, id string) {
	if screen == nil || whitePixel == nil { logf("ERRO (%s): screen/whitePixel nil", id); return }
	if thickness < 1 { thickness = 1 }
	dx := x2 - x1; dy := y2 - y1; lengthSq := dx*dx + dy*dy
	if lengthSq < 0.01 { return } // Linha muito curta.
	length := float32(math.Sqrt(float64(lengthSq)))
	nx := dx / length; ny := dy / length // Vetor normalizado.
	if math.IsNaN(float64(nx)) || math.IsNaN(float64(ny)) || math.IsInf(float64(nx), 0) || math.IsInf(float64(ny), 0) { logf("ERRO (%s): NaN/Inf norm vec", id); return }
	halfThick := thickness / 2.0; px := -ny * halfThick; py := nx * halfThick // Vetor perpendicular.
	// Vértices do retângulo.
	v1x, v1y := x1+px, y1+py; v2x, v2y := x1-px, y1-py; v3x, v3y := x2-px, y2-py; v4x, v4y := x2+px, y2+py
	// Validação dos vértices.
	coords := []float32{v1x, v1y, v2x, v2y, v3x, v3y, v4x, v4y}; for i, val := range coords { if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) { logf("ERRO (%s): NaN/Inf vert coord [%d]", id, i); return } }
	// Cor para float32.
	r, gVal, b, a := clr.RGBA(); colorR, colorG, colorB, colorA := float32(r)/65535.0, float32(gVal)/65535.0, float32(b)/65535.0, float32(a)/65535.0
	// Definição dos vértices Ebiten (usando textura 1x1).
	vertices := []ebiten.Vertex{ {DstX: v1x, DstY: v1y, SrcX: 0, SrcY: 0, ColorR: colorR, ColorG: colorG, ColorB: colorB, ColorA: colorA}, {DstX: v2x, DstY: v2y, SrcX: 0, SrcY: 0, ColorR: colorR, ColorG: colorG, ColorB: colorB, ColorA: colorA}, {DstX: v3x, DstY: v3y, SrcX: 0, SrcY: 0, ColorR: colorR, ColorG: colorG, ColorB: colorB, ColorA: colorA}, {DstX: v4x, DstY: v4y, SrcX: 0, SrcY: 0, ColorR: colorR, ColorG: colorG, ColorB: colorB, ColorA: colorA}, }
	indices := []uint16{0, 1, 2, 0, 2, 3} // Índices dos triângulos.
	op := &ebiten.DrawTrianglesOptions{AntiAlias: true} // Habilita suavização.
	// Desenha na tela.
	screen.DrawTriangles(vertices, indices, whitePixel, op)
}

// --- Função Principal ---

// main configura e inicia o jogo.
func main() {
	gameInstance := NewGame() // Cria a instância do jogo.
	ebiten.SetWindowSize(gameInstance.screenWidth, gameInstance.screenHeight) // Define tamanho inicial.
	ebiten.SetWindowTitle("Malha Ferroviária Interativa (v8.4 - Documentado)") // Define título.
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled) // Permite redimensionar.
	logln("Iniciando loop...") // Mensagem de início.
	// Roda o jogo. Bloqueia até fechar.
	if err := ebiten.RunGame(gameInstance); err != nil {
		if err != ebiten.Termination { // Se não for um fechamento normal...
			logf("Erro fatal: %v", err) // ...loga o erro.
		} else {
			logln("Aplicativo terminado.") // Loga término normal.
		}
	}
	logln("==== Fim ====") // Mensagem final.
}