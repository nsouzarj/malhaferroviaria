package main

import (
	// Pacotes padrão do Go
	"encoding/json" // Para codificar/decodificar dados em formato JSON (salvar/carregar)
	"fmt"           // Para formatação de strings (usado em logs e UI)
	"image"         // Para manipulação básica de imagens (usado em retângulos do popup)
	"image/color"   // Para definições de cores
	"log"           // Para registrar mensagens (erros, informações)
	"math"          // Para funções matemáticas (Sqrt, Pow, Max, Min, IsNaN, IsInf)
	"os"            // Para interação com o sistema operacional (arquivos)
	"strings"       // Para manipulação de strings (usado em HasSuffix)

	// Biblioteca Ebitengine e seus submódulos
	"github.com/hajimehoshi/ebiten/v2"          // Núcleo do Ebitengine
	"github.com/hajimehoshi/ebiten/v2/ebitenutil" // Utilitários (DebugPrint, LoadImage)
	"github.com/hajimehoshi/ebiten/v2/inpututil"  // Utilitários para entrada (teclado, mouse)
	"github.com/hajimehoshi/ebiten/v2/text"      // Para desenhar texto na tela
	"github.com/hajimehoshi/ebiten/v2/vector"    // Para desenho vetorial (retângulos, círculos)

	// Biblioteca externa para diálogos de arquivo nativos
	"github.com/sqweek/dialog"

	// Fonte básica para texto simples
	"golang.org/x/image/font/basicfont"
)

// --- Constantes Globais ---

const (
	// pixelsPerMeter define a escala do mundo.
	// 0.01 significa que 0.01 pixels na tela representam 1 metro no mundo,
	// ou, inversamente, 1 pixel na tela representa 1 / 0.01 = 100 metros.
	pixelsPerMeter = 0.01

	// cameraScrollSpeed define quantos pixels a câmera se move por frame quando as setas são pressionadas.
	cameraScrollSpeed = 5.0

	// Dimensões e aparência do menu popup.
	popupWidth           = 100 // Largura em pixels.
	popupOptionHeight    = 20  // Altura de cada opção de texto (ex: "Apagar").
	popupPadding         = 5   // Espaçamento interno (margem).
	popupColorSquareSize = 16  // Tamanho (largura e altura) dos quadrados de seleção de cor.

	// hitThreshold define a distância máxima em pixels que um clique pode estar de uma linha
	// para ser considerado um clique "na" linha (para abrir o popup).
	hitThreshold = 8.0

	// nodeRadiusFactor é usado para calcular o raio dos círculos (nós/estações)
	// nos endpoints das linhas, baseado na espessura da linha.
	// Raio = (Espessura * Fator) / 2.
	nodeRadiusFactor = 1.5

	// nodeOutlineWidth define a espessura (em pixels) do contorno dos nós/estações.
	nodeOutlineWidth = 1.0

	// tooltipPadding define o espaçamento interno (margem) do balão de dica (tooltip).
	tooltipPadding = 4
)

// --- Estruturas de Dados ---

// Track representa um único segmento de trilho na malha ferroviária.
type Track struct {
	// Coordenadas X e Y dos pontos inicial (1) e final (2) do trilho.
	// IMPORTANTE: Estas são coordenadas do "MUNDO", não da tela. Podem ser valores grandes.
	// Os campos precisam começar com letra maiúscula para serem exportados e salvos em JSON.
	// As tags `json:"..."` especificam o nome do campo no arquivo JSON (geralmente minúsculo).
	X1 float64 `json:"x1"`
	Y1 float64 `json:"y1"`
	X2 float64 `json:"x2"`
	Y2 float64 `json:"y2"`

	// Cor do trilho. Usa o tipo color.RGBA que o `encoding/json` sabe como serializar/desserializar.
	Color color.RGBA `json:"color"`

	// Espessura visual do trilho, em pixels.
	Thickness float64 `json:"thickness"`

	// Comprimento do trilho calculado em metros, baseado na escala `pixelsPerMeter`.
	// É armazenado para evitar recálculo constante.
	LengthMeters float64 `json:"lengthMeters"`
}

// PopupOption representa uma única opção (botão) dentro do menu popup contextual.
type PopupOption struct {
	// Texto a ser exibido na opção (ex: "Apagar"). Pode ser vazio se for só uma cor.
	Label string
	// Rect define a área retangular clicável desta opção na tela.
	// As coordenadas são relativas à posição *original* onde o popup foi aberto (g.popupX, g.popupY).
	Rect image.Rectangle
	// Color (opcional) armazena um ponteiro para a cor se esta opção for uma amostra de cor.
	// Usado para desenhar o quadrado colorido. É um ponteiro para evitar cópia desnecessária.
	Color *color.RGBA
	// Action é a função que será chamada quando esta opção for clicada.
	// Usa uma closure para capturar o estado necessário (ex: índice do trilho) no momento da criação.
	Action func()
}

// Game contém todo o estado da aplicação.
type Game struct {
	// tracks é a slice (lista dinâmica) que armazena todos os trilhos desenhados.
	tracks []Track

	// Coordenadas do MUNDO onde o desenho atual começou (quando o botão esquerdo foi pressionado).
	startX, startY float64
	// drawing indica se o usuário está atualmente arrastando o mouse para desenhar um novo trilho.
	drawing bool

	// Cor e espessura atualmente selecionadas para *novos* trilhos.
	currentColor color.RGBA
	thickness    float64

	// Dimensões atuais da janela (em pixels). Atualizadas pelo método Layout.
	screenWidth  int
	screenHeight int

	// whitePixel é uma imagem de 1x1 pixel branca usada como textura base para
	// desenhar triângulos coloridos (linhas). Isso contorna um bug/problema
	// observado anteriormente ao passar `nil` para DrawTriangles.
	whitePixel *ebiten.Image

	// Mapeamentos para seleção de cor dos trilhos via teclado.
	colorPalette map[ebiten.Key]color.RGBA // Associa teclas (Key1, Key2...) a cores RGBA.
	colorNames   map[ebiten.Key]string    // Associa teclas a nomes de cores para logging.

	// cameraOffsetX armazena o deslocamento horizontal da câmera.
	// Valor positivo significa que o mundo foi movido para a esquerda (vemos áreas com X maior).
	// Valor negativo significa que o mundo foi movido para a direita.
	cameraOffsetX float64

	// Cor de fundo atual da tela.
	backgroundColor color.RGBA

	// Estado do menu popup contextual.
	popupVisible       bool          // Indica se o popup está sendo exibido.
	popupX, popupY     int           // Coordenadas originais na TELA onde o popup foi aberto (clique direito).
	selectedTrackIndex int           // Índice (na slice `g.tracks`) do trilho selecionado que o popup edita (-1 se nenhum).
	popupOptions       []PopupOption // Slice contendo as opções atualmente visíveis no popup.

	// hoveredTrackIndex armazena o índice do trilho que está atualmente sob o cursor do mouse (-1 se nenhum).
	// Usado para exibir o tooltip com a metragem.
	hoveredTrackIndex int
}

// --- Funções de Inicialização e Helpers ---

// NewGame cria e inicializa uma nova instância do estado do jogo.
func NewGame() *Game {
	// Obtém o tamanho do monitor principal.
	monitorWidth, monitorHeight := ebiten.Monitor().Size()
	// Define um tamanho de janela padrão ou 90% do monitor.
	if monitorWidth <= 0 || monitorHeight <= 0 {
		monitorWidth, monitorHeight = 1024, 768
	} else {
		monitorWidth, monitorHeight = int(float64(monitorWidth)*0.9), int(float64(monitorHeight)*0.9)
	}
	fmt.Printf("Tamanho: %dx%d | Escala: 1 pixel = %.0f metros\n", monitorWidth, monitorHeight, 1.0/pixelsPerMeter)

	// Configura o logging para um arquivo "game.log". Trunca o arquivo anterior a cada execução.
	logFile, err := os.OpenFile("game.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND|os.O_TRUNC, 0660)
	if err == nil {
		log.SetOutput(logFile)                                   // Direciona a saída do log para o arquivo.
		log.SetFlags(log.LstdFlags | log.Lmicroseconds)         // Adiciona data, hora e microssegundos aos logs.
		log.Println("==== Log (v7.3 - Correct Formatting) ====") // Mensagem inicial do log.
	} else {
		log.Println("Erro ao configurar log para arquivo:", err) // Loga erro se não puder abrir o arquivo.
	}

	// Cria a imagem de 1x1 pixel branco para a textura base.
	whiteImg := ebiten.NewImage(1, 1)
	whiteImg.Fill(color.White)

	// Define a paleta de cores e seus nomes associados às teclas 1-4.
	palette := map[ebiten.Key]color.RGBA{
		ebiten.Key1: {255, 0, 0, 255}, ebiten.Key2: {0, 0, 255, 255}, ebiten.Key3: {255, 255, 0, 255}, ebiten.Key4: {0, 255, 0, 255},
	}
	names := map[ebiten.Key]string{
		ebiten.Key1: "Vermelho", ebiten.Key2: "Azul", ebiten.Key3: "Amarelo", ebiten.Key4: "Verde",
	}
	initialColorKey := ebiten.Key1 // Começa com Vermelho selecionado.

	// Retorna a struct Game inicializada com os valores padrão.
	return &Game{
		tracks:             []Track{},                      // Lista de trilhos começa vazia.
		currentColor:       palette[initialColorKey],       // Cor inicial definida.
		thickness:          5.0,                            // Espessura inicial.
		screenWidth:        monitorWidth,                   // Dimensões da janela.
		screenHeight:       monitorHeight,                  //
		whitePixel:         whiteImg,                       // Referência à textura 1x1.
		colorPalette:       palette,                        // Mapa da paleta.
		colorNames:         names,                          // Mapa dos nomes das cores.
		cameraOffsetX:      0.0,                            // Câmera começa na posição 0.
		backgroundColor:    color.RGBA{0, 0, 0, 255},       // Fundo inicial preto.
		popupVisible:       false,                          // Popup começa invisível.
		selectedTrackIndex: -1,                            // Nenhum trilho selecionado inicialmente.
		hoveredTrackIndex:  -1,                            // Nenhum trilho sob o mouse inicialmente.
		popupOptions:       []PopupOption{},                // Lista de opções do popup começa vazia.
	}
}

// screenToWorld converte coordenadas da Tela (pixels relativos à janela)
// para coordenadas do Mundo (pixels absolutos no espaço de desenho).
func (g *Game) screenToWorld(screenX, screenY int) (float64, float64) {
	// Adiciona o deslocamento da câmera ao X da tela. Y não tem scroll neste exemplo.
	worldX := float64(screenX) + g.cameraOffsetX
	worldY := float64(screenY)
	return worldX, worldY
}

// worldToScreen converte coordenadas do Mundo para coordenadas da Tela.
func (g *Game) worldToScreen(worldX, worldY float64) (float32, float32) {
	// Subtrai o deslocamento da câmera do X do mundo.
	screenX := float32(worldX - g.cameraOffsetX)
	screenY := float32(worldY)
	return screenX, screenY
}

// calculateLengthMeters calcula o comprimento de um segmento de linha (em coordenadas do mundo)
// e converte para metros usando a constante `pixelsPerMeter`.
func calculateLengthMeters(x1, y1, x2, y2 float64) float64 {
	dx := x2 - x1 // Diferença em X (pixels do mundo)
	dy := y2 - y1 // Diferença em Y (pixels do mundo)
	pixelLength := math.Sqrt(dx*dx + dy*dy) // Comprimento em pixels (Teorema de Pitágoras)
	if pixelsPerMeter <= 0 {                 // Proteção contra divisão por zero/infinito
		return pixelLength // Retorna em pixels se a escala for inválida
	}
	// Divide o comprimento em pixels pela escala (pixels por metro) para obter metros.
	// Se pixelsPerMeter < 1 (ex: 0.01), a divisão aumenta o valor, resultando em mais metros.
	return pixelLength / pixelsPerMeter
}

// saveTracks abre um diálogo para o usuário escolher um local e nome de arquivo,
// depois salva a slice `g.tracks` nesse arquivo em formato JSON.
func (g *Game) saveTracks() error {
	// Mostra o diálogo de "Salvar como..."
	savePath, err := dialog.File().Filter("JSON Malha", "json").Title("Salvar Malha Ferroviária").Save()
	if err != nil {
		if err == dialog.ErrCancelled { // Se o usuário cancelou.
			log.Println("Salvar cancelado.")
			return nil // Não é um erro da aplicação.
		}
		// Outro erro ao mostrar o diálogo.
		log.Printf("ERRO diálogo salvar: %v", err)
		return err
	}
	// Segurança extra: verifica se um caminho foi realmente retornado.
	if len(savePath) == 0 {
		log.Println("Salvar cancelado (caminho vazio).")
		return nil
	}
	// Garante que o arquivo tenha a extensão .json.
	if !strings.HasSuffix(strings.ToLower(savePath), ".json") {
		savePath += ".json"
	}

	// Cria (ou sobrescreve) o arquivo no caminho escolhido.
	file, err := os.Create(savePath)
	if err != nil {
		log.Printf("ERRO criar '%s': %v", savePath, err)
		return err
	}
	// Garante que o arquivo será fechado ao final da função.
	defer file.Close()

	// Cria um codificador JSON que escreve no arquivo.
	encoder := json.NewEncoder(file)
	// Formata o JSON com indentação para facilitar a leitura humana.
	encoder.SetIndent("", "  ")
	// Codifica a slice g.tracks para o formato JSON e escreve no arquivo.
	err = encoder.Encode(g.tracks)
	// Verifica se houve erro durante a codificação.
	if err != nil {
		log.Printf("ERRO codificar JSON '%s': %v", savePath, err)
		return err // Retorna o erro.
	}

	// Se chegou aqui, tudo correu bem.
	log.Printf("Salvo: '%s' (%d trilhos)", savePath, len(g.tracks))
	return nil // Retorna nil indicando sucesso.
}

// loadTracks abre um diálogo para o usuário escolher um arquivo JSON,
// depois lê e decodifica os dados desse arquivo para a slice `g.tracks`.
func (g *Game) loadTracks() error {
	// Mostra o diálogo de "Abrir arquivo...".
	loadPath, err := dialog.File().Filter("JSON Malha", "json").Title("Carregar Malha Ferroviária").Load()
	if err != nil {
		if err == dialog.ErrCancelled { // Se o usuário cancelou.
			log.Println("Carregar cancelado.")
			return nil // Não é um erro da aplicação.
		}
		// Outro erro ao mostrar o diálogo.
		log.Printf("ERRO diálogo carregar: %v", err)
		return err
	}
	// Segurança extra: verifica se um caminho foi retornado.
	if len(loadPath) == 0 {
		log.Println("Carregar cancelado (caminho vazio).")
		return nil
	}

	// Abre o arquivo escolhido para leitura.
	file, err := os.Open(loadPath)
	if err != nil {
		log.Printf("ERRO abrir '%s': %v", loadPath, err)
		return err
	}
	// Garante que o arquivo será fechado ao final da função.
	defer file.Close()

	// Variável temporária para armazenar os trilhos carregados.
	var loadedTracks []Track
	// Cria um decodificador JSON que lê do arquivo.
	decoder := json.NewDecoder(file)
	// Decodifica os dados JSON do arquivo para a slice loadedTracks.
	err = decoder.Decode(&loadedTracks)
	// Verifica se houve erro durante a decodificação.
	if err != nil {
		log.Printf("ERRO decodificar JSON '%s': %v", loadPath, err)
		return err // Retorna o erro.
	}

	// Se chegou aqui, a decodificação foi bem-sucedida.
	// Log para diagnóstico: quantos trilhos foram lidos e as coordenadas do primeiro.
	log.Printf("Decodificação JSON OK. %d trilhos lidos.", len(loadedTracks))
	if len(loadedTracks) > 0 {
		log.Printf("  -> Coords Mundo 1º trilho carregado: (%.1f, %.1f) -> (%.1f, %.1f)", loadedTracks[0].X1, loadedTracks[0].Y1, loadedTracks[0].X2, loadedTracks[0].Y2)
	} else {
		log.Println("  -> Nenhum trilho carregado.")
	}
	// Substitui os trilhos atuais pelos carregados.
	g.tracks = loadedTracks
	// Reseta a posição da câmera para o início.
	g.cameraOffsetX = 0
	log.Printf("Malha carregada e câmera resetada: '%s' (%d trilhos)", loadPath, len(g.tracks))
	return nil // Retorna nil indicando sucesso.
}

// pointSegmentDistance calcula a menor distância entre um ponto (px, py)
// e um segmento de linha definido por (ax, ay) e (bx, by).
func pointSegmentDistance(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay           // Vetor diretor do segmento AB
	lengthSq := dx*dx + dy*dy      // Quadrado do comprimento do segmento
	if lengthSq == 0 {             // Se o segmento for um ponto (A == B)
		// Distância entre P e A
		return math.Sqrt(math.Pow(px-ax, 2) + math.Pow(py-ay, 2))
	}
	// Calcula a projeção do vetor AP no vetor AB.
	// t representa a posição do ponto projetado ao longo do segmento (0=A, 1=B).
	t := ((px-ax)*dx + (py-ay)*dy) / lengthSq
	// Limita t entre 0 e 1 para garantir que o ponto projetado esteja *dentro* do segmento.
	t = math.Max(0, math.Min(1, t))
	// Calcula as coordenadas do ponto mais próximo no segmento.
	closestX := ax + t*dx
	closestY := ay + t*dy
	// Calcula a distância entre o ponto P e o ponto mais próximo no segmento.
	return math.Sqrt(math.Pow(px-closestX, 2) + math.Pow(py-closestY, 2))
}

// findClosestTrack encontra o índice do trilho mais próximo de um ponto (em coordenadas do mundo),
// desde que a distância seja menor que `hitThreshold` (ajustado pela espessura do trilho).
func (g *Game) findClosestTrack(worldX, worldY float64) int {
	closestIndex := -1                     // Índice do trilho mais próximo (-1 se nenhum).
	minDist := hitThreshold               // Menor distância encontrada até agora.
	// Itera pelos trilhos de trás para frente (último desenhado -> primeiro desenhado).
	// Isso faz com que trilhos desenhados por cima sejam selecionados primeiro em caso de sobreposição.
	for i := len(g.tracks) - 1; i >= 0; i-- {
		track := g.tracks[i]
		// Calcula a distância do ponto ao segmento do trilho atual.
		dist := pointSegmentDistance(worldX, worldY, track.X1, track.Y1, track.X2, track.Y2)
		// Calcula um threshold efetivo que considera metade da espessura do trilho.
		effectiveThreshold := hitThreshold + track.Thickness/2.0
		// Se a distância for menor que a menor encontrada *e* menor que o threshold efetivo...
		if dist < minDist && dist < effectiveThreshold {
			minDist = dist         // Atualiza a menor distância.
			closestIndex = i     // Armazena o índice deste trilho.
		}
	}
	return closestIndex // Retorna o índice encontrado ou -1.
}

// --- Métodos da Interface ebiten.Game ---

// Update é chamado a cada tick lógico do jogo (normalmente 60 vezes por segundo).
// É responsável por processar entradas, atualizar o estado do jogo e a lógica.
func (g *Game) Update() error {
	// Obtém a posição atual do cursor na tela.
	cursorX, cursorY := ebiten.CursorPosition()
	// Converte a posição do cursor para coordenadas do mundo.
	worldCursorX, worldCursorY := g.screenToWorld(cursorX, cursorY)
	// Flag para indicar se um clique foi "consumido" pela lógica do popup.
	popupClicked := false

	// --- Tooltip Hover Check ---
	// Verifica qual trilho está sob o mouse, mas apenas se o popup não estiver visível.
	if !g.popupVisible {
		g.hoveredTrackIndex = g.findClosestTrack(worldCursorX, worldCursorY)
	} else {
		// Esconde o tooltip se o popup estiver ativo.
		g.hoveredTrackIndex = -1
	}

	// --- Lógica de Interação com o Popup (Prioritária) ---
	if g.popupVisible {
		clickPoint := image.Pt(cursorX, cursorY)                 // Ponto do clique na tela.
		popupDrawX, popupDrawY := g.calculatePopupDrawPosition() // Posição onde o popup é *desenhado*.

		// Verifica clique esquerdo para selecionar uma opção.
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			clickedOnOption := false
			// Itera pelas opções definidas no popup.
			for _, option := range g.popupOptions {
				// Calcula a área real de clique da opção na tela (considerando o ajuste de posição).
				optionDrawRect := option.Rect.Add(image.Pt(popupDrawX-g.popupX, popupDrawY-g.popupY))
				// Se o ponto clicado está dentro da área da opção...
				if clickPoint.In(optionDrawRect) {
					option.Action()             // Executa a ação associada à opção.
					g.popupVisible = false      // Fecha o popup.
					g.selectedTrackIndex = -1   // Reseta a seleção.
					popupClicked = true         // Marca que o clique foi consumido.
					clickedOnOption = true      // Marca que foi numa opção.
					break                       // Sai do loop de opções.
				}
			}
			// Se clicou fora de todas as opções, fecha o popup.
			if !clickedOnOption {
				g.popupVisible = false
				g.selectedTrackIndex = -1
				popupClicked = true // Clicar fora também consome o clique.
			}
		}

		// Verifica clique direito (fora das opções) para fechar o popup.
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
			inAnyOption := false
			for _, option := range g.popupOptions {
				optionDrawRect := option.Rect.Add(image.Pt(popupDrawX-g.popupX, popupDrawY-g.popupY))
				if clickPoint.In(optionDrawRect) {
					inAnyOption = true
					break
				}
			}
			// Se não clicou em nenhuma opção, fecha o popup.
			if !inAnyOption {
				g.popupVisible = false
				g.selectedTrackIndex = -1
				popupClicked = true // Consome o clique direito.
			}
		}
	}

	// --- Outras Interações (Só processa se o clique não foi consumido pelo popup) ---
	if !popupClicked {
		// Abrir Popup com clique direito.
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
			clickedIndex := g.findClosestTrack(worldCursorX, worldCursorY)
			// Se encontrou um trilho próximo...
			if clickedIndex != -1 {
				g.selectedTrackIndex = clickedIndex                // Armazena o índice.
				g.popupVisible = true                              // Torna o popup visível.
				g.popupX, g.popupY = cursorX, cursorY              // Armazena a posição *original* do clique na tela.
				g.generatePopupOptions()                           // Cria as opções do menu para este trilho.
				g.hoveredTrackIndex = -1                           // Garante que o tooltip não apareça junto.
			} else { // Clicou direito no vazio.
				g.popupVisible = false                             // Garante que o popup feche.
				g.selectedTrackIndex = -1
			}
		}

		// Iniciar Desenho com clique esquerdo.
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			g.startX, g.startY = worldCursorX, worldCursorY // Armazena ponto inicial (MUNDO).
			g.drawing = true                               // Ativa flag de desenho.
			g.popupVisible = false                         // Fecha o popup se estiver aberto.
			g.selectedTrackIndex = -1
			g.hoveredTrackIndex = -1                        // Esconde tooltip.
		}
		// Finalizar Desenho ao soltar o botão esquerdo.
		if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) && g.drawing {
			endWorldX, endWorldY := worldCursorX, worldCursorY // Pega ponto final (MUNDO).
			// Verifica se o ponto inicial era válido (não NaN).
			if !math.IsNaN(g.startX) && !math.IsNaN(g.startY) {
				// Calcula a distância em pixels.
				distPx := math.Sqrt(math.Pow(endWorldX-g.startX, 2) + math.Pow(endWorldY-g.startY, 2))
				// Só adiciona se for maior que 1 pixel (evita pontos).
				if distPx > 1.0 {
					// Calcula o comprimento em metros.
					lengthM := calculateLengthMeters(g.startX, g.startY, endWorldX, endWorldY)
					// Verifica se o cálculo do comprimento é válido.
					if !math.IsNaN(lengthM) {
						// Adiciona o novo trilho à slice.
						g.tracks = append(g.tracks, Track{
							X1:           g.startX, // Coordenadas do MUNDO.
							Y1:           g.startY,
							X2:           endWorldX,
							Y2:           endWorldY,
							Color:        g.currentColor, // Cor atual selecionada.
							Thickness:    g.thickness,    // Espessura atual.
							LengthMeters: lengthM,        // Comprimento calculado.
						})
					} else {
						log.Printf("WARN: Comprimento NaN ao tentar adicionar trilho.")
					}
				}
			} else {
				log.Printf("WARN: Tentativa de finalizar desenho sem ponto inicial válido.")
			}
			g.drawing = false               // Desativa flag de desenho.
			g.startX = math.NaN()          // Invalida ponto inicial para próxima vez.
			g.startY = math.NaN()
		}
	}

	// --- Controles Gerais ---

	// Controle da Câmera com setas esquerda/direita.
	if ebiten.IsKeyPressed(ebiten.KeyLeft) {
		g.cameraOffsetX -= cameraScrollSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) {
		g.cameraOffsetX += cameraScrollSpeed
	}

	// Seleção de Cor dos Trilhos com teclas 1-4.
	for key, clr := range g.colorPalette {
		if inpututil.IsKeyJustPressed(key) { // Verifica se a tecla foi *acabada* de pressionar.
			if g.currentColor != clr {       // Muda a cor apenas se for diferente da atual.
				g.currentColor = clr
				log.Printf("Cor trilho: %s", g.colorNames[key])
			}
			break // Sai do loop assim que encontrar uma tecla pressionada.
		}
	}

	// Seleção de Cor de Fundo com teclas F1-F4.
	if inpututil.IsKeyJustPressed(ebiten.KeyF1) {
		g.backgroundColor = color.RGBA{0, 0, 0, 255} // Preto
		log.Println("Fundo: Preto")
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF2) {
		g.backgroundColor = color.RGBA{50, 50, 50, 255} // Cinza Escuro
		log.Println("Fundo: Cinza Escuro")
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF3) {
		g.backgroundColor = color.RGBA{100, 100, 120, 255} // Cinza Azulado
		log.Println("Fundo: Cinza Azulado")
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF4) {
		g.backgroundColor = color.RGBA{240, 240, 240, 255} // Branco Gelo
		log.Println("Fundo: Branco Gelo")
	}

	// Ajustar espessura com +/- (e numpad +/-).
	prevThickness := g.thickness
	if inpututil.IsKeyJustPressed(ebiten.KeyEqual) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadAdd) {
		g.thickness += 1.0
		if g.thickness > 50 { // Limite máximo.
			g.thickness = 50
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyMinus) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadSubtract) {
		g.thickness -= 1.0
		if g.thickness < 1 { // Limite mínimo.
			g.thickness = 1
		}
	}
	// Loga apenas se a espessura mudou.
	if g.thickness != prevThickness {
		log.Printf("Espessura: %.1f px", g.thickness)
	}

	// Limpar a malha com 'C'.
	if inpututil.IsKeyJustPressed(ebiten.KeyC) {
		g.tracks = []Track{}             // Limpa a slice de trilhos.
		g.cameraOffsetX = 0             // Reseta a câmera.
		g.popupVisible = false           // Esconde o popup.
		g.selectedTrackIndex = -1
		g.hoveredTrackIndex = -1         // Reseta hover.
		log.Println("Malha limpa.")
	}

	// Salvar / Carregar com 'S' / 'L'.
	if inpututil.IsKeyJustPressed(ebiten.KeyS) {
		g.saveTracks() // Chama a função de salvar.
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyL) {
		g.loadTracks() // Chama a função de carregar.
	}

	// Sair do jogo com 'ESC'.
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		log.Println("Saindo.")
		return ebiten.Termination // Sinaliza para o Ebiten encerrar.
	}

	// Se nenhuma ação de encerramento foi tomada, retorna nil para continuar.
	return nil
}

// generatePopupOptions cria a lista de opções para o menu popup
// baseado no `g.selectedTrackIndex` atual.
func (g *Game) generatePopupOptions() {
	g.popupOptions = []PopupOption{} // Limpa opções anteriores.
	// Verifica se o índice selecionado é válido.
	if g.selectedTrackIndex < 0 || g.selectedTrackIndex >= len(g.tracks) {
		return
	}

	// --- Opções de Cor ---
	colorKeys := []ebiten.Key{ebiten.Key1, ebiten.Key2, ebiten.Key3, ebiten.Key4} // Ordem desejada.
	// Calcula Y e X inicial para a linha de quadrados de cor, baseado na posição original do popup.
	colorRowY := g.popupY + popupPadding
	colorStartX := g.popupX + popupPadding
	squareSpacing := popupColorSquareSize + 5 // Espaço entre quadrados.

	// Cria uma opção para cada cor na paleta.
	for i, key := range colorKeys {
		optColor := g.colorPalette[key] // Cor desta opção.
		// IMPORTANTE: Captura o índice atual para a closure.
		// Se usássemos g.selectedTrackIndex diretamente na closure, ele usaria o valor *mais recente*
		// de g.selectedTrackIndex quando a ação fosse executada, o que poderia estar errado.
		optIndex := g.selectedTrackIndex

		// Define o retângulo clicável para este quadrado de cor.
		optionRect := image.Rect(
			colorStartX+i*squareSpacing,             // X inicial
			colorRowY,                               // Y inicial
			colorStartX+i*squareSpacing+popupColorSquareSize, // X final
			colorRowY+popupColorSquareSize,          // Y final
		)
		// Adiciona a opção à lista.
		g.popupOptions = append(g.popupOptions, PopupOption{
			Rect:  optionRect,    // Área clicável.
			Color: &optColor,     // Ponteiro para a cor (para desenho).
			Action: func() {      // A função a ser executada no clique.
				// Verificação dupla de validade do índice dentro da closure (segurança).
				if optIndex >= 0 && optIndex < len(g.tracks) {
					g.tracks[optIndex].Color = optColor // Muda a cor do trilho correto.
					log.Printf("Cor do trilho %d -> %s", optIndex, g.colorNames[key])
				} else {
					log.Printf("WARN: Índice inválido %d ao tentar mudar cor via popup", optIndex)
				}
			},
		})
	}

	// --- Opção Apagar ---
	// Calcula a posição Y para o botão "Apagar" (abaixo dos quadrados de cor).
	deleteRowY := colorRowY + popupColorSquareSize + popupPadding
	// Define o retângulo clicável para o botão "Apagar".
	deleteRect := image.Rect(
		g.popupX+popupPadding,              // X inicial (com padding).
		deleteRowY,                         // Y inicial.
		g.popupX+popupWidth-popupPadding,  // X final (largura total menos padding).
		deleteRowY+popupOptionHeight,       // Y final.
	)
	// Captura o índice atual para a closure.
	optIndex := g.selectedTrackIndex
	// Adiciona a opção "Apagar" à lista.
	g.popupOptions = append(g.popupOptions, PopupOption{
		Label: "Apagar",        // Texto do botão.
		Rect:  deleteRect,    // Área clicável.
		Action: func() {        // Ação a ser executada.
			// Verificação dupla de validade do índice.
			if optIndex >= 0 && optIndex < len(g.tracks) {
				log.Printf("Apagando trilho %d", optIndex)
				// Técnica padrão para remover um elemento de uma slice em Go.
				g.tracks = append(g.tracks[:optIndex], g.tracks[optIndex+1:]...)
				g.selectedTrackIndex = -1 // Reseta a seleção, pois os índices podem ter mudado.
			} else {
				log.Printf("WARN: Índice inválido %d ao tentar apagar via popup", optIndex)
			}
		},
	})
}

// calculatePopupDrawPosition calcula a posição (X, Y) onde o popup deve ser
// efetivamente desenhado na tela, ajustando a posição original (`g.popupX`, `g.popupY`)
// para garantir que o popup não saia dos limites da tela.
func (g *Game) calculatePopupDrawPosition() (int, int) {
	// Calcula a altura necessária para o popup baseado nas opções geradas.
	popupHeight := 0
	if len(g.popupOptions) > 0 {
		maxY := 0 // Encontra o Y máximo ocupado por qualquer opção.
		for _, opt := range g.popupOptions {
			if opt.Rect.Max.Y > maxY {
				maxY = opt.Rect.Max.Y
			}
		}
		// A altura é a diferença entre o Y máximo e o Y original do popup, mais um padding final.
		popupHeight = (maxY - g.popupY) + popupPadding
	} else {
		// Altura de fallback caso não haja opções (não deveria acontecer se popupVisible=true).
		popupHeight = popupPadding*2 + popupColorSquareSize + popupOptionHeight + popupPadding
	}

	// Começa com a posição original do clique.
	drawPopupX := g.popupX
	drawPopupY := g.popupY

	// Ajusta o X se o popup sair pela direita.
	if drawPopupX+popupWidth > g.screenWidth {
		drawPopupX = g.screenWidth - popupWidth
	}
	// Ajusta o Y se o popup sair por baixo.
	if drawPopupY+popupHeight > g.screenHeight {
		drawPopupY = g.screenHeight - popupHeight
	}
	// Ajusta o X se sair pela esquerda (menos comum, mas possível).
	if drawPopupX < 0 {
		drawPopupX = 0
	}
	// Ajusta o Y se sair por cima.
	if drawPopupY < 0 {
		drawPopupY = 0
	}
	// Retorna a posição ajustada.
	return drawPopupX, drawPopupY
}

// drawNode é uma função helper para desenhar um nó (círculo com contorno).
func drawNode(screen *ebiten.Image, x, y, radius, strokeWidth float32, fillColor, outlineColor color.Color) {
	// Não tenta desenhar se o raio for inválido.
	if radius <= 0 {
		return
	}
	// Desenha o círculo preenchido.
	vector.DrawFilledCircle(screen, x, y, radius, fillColor, true)
	// Desenha o contorno apenas se a espessura for positiva.
	if strokeWidth > 0 {
		vector.StrokeCircle(screen, x, y, radius, strokeWidth, outlineColor, true)
	}
}

// Draw é chamado a cada frame para desenhar a cena na tela.
func (g *Game) Draw(screen *ebiten.Image) {
	// Verificações essenciais.
	if screen == nil || g.whitePixel == nil {
		log.Println("ERRO CRÍTICO: screen/whitePixel nil em Draw")
		return
	}
	// Limpa a tela com a cor de fundo atual.
	screen.Fill(g.backgroundColor)

	// Variável para acumular o comprimento total.
	totalLengthMeters := 0.0
	// Posição atual do cursor (usada para tooltip).
	cursorX, cursorY := ebiten.CursorPosition()

	// --- Desenhar Trilhos e Nós ---
	// Itera sobre todos os trilhos armazenados.
	for i, track := range g.tracks {
		// Acumula comprimento total (ignorando NaN).
		if !math.IsNaN(track.LengthMeters) {
			totalLengthMeters += track.LengthMeters
		}
		// Converte coordenadas do mundo para coordenadas da tela.
		screenX1, screenY1 := g.worldToScreen(track.X1, track.Y1)
		screenX2, screenY2 := g.worldToScreen(track.X2, track.Y2)

		// Culling: Otimização simples para não desenhar trilhos completamente fora da tela.
		maxScreenX := float32(g.screenWidth + 100) // Margem direita/esquerda.
		minScreenX := float32(-100)
		// Se ambos os pontos X estão muito à esquerda OU muito à direita, pula o desenho.
		if (screenX1 < minScreenX && screenX2 < minScreenX) || (screenX1 > maxScreenX && screenX2 > maxScreenX) {
			continue // Pula para o próximo trilho no loop.
		}

		// Determina as cores para desenhar a linha e os nós.
		lineDrawColor := track.Color   // Cor normal da linha.
		nodeFillColor := track.Color   // Cor normal de preenchimento do nó.
		nodeOutlineColor := color.White // Contorno normal branco.

		// Se o popup estiver visível E este for o trilho selecionado...
		if g.popupVisible && i == g.selectedTrackIndex {
			// ...usa cores de destaque.
			lineDrawColor = color.RGBA{R: 255, G: 255, B: 255, A: 255} // Linha branca.
			nodeFillColor = color.RGBA{R: 255, G: 255, B: 255, A: 255} // Nó branco.
			nodeOutlineColor = color.Black                            // Contorno preto.
		}

		// Desenha a linha principal do trilho.
		drawThickLine(screen, g.whitePixel, screenX1, screenY1, screenX2, screenY2, float32(track.Thickness), lineDrawColor, "track")

		// Calcula o raio dos nós baseado na espessura da linha.
		nodeRadius := float32(track.Thickness*nodeRadiusFactor / 2.0)
		// Garante um raio mínimo para visibilidade.
		if nodeRadius < 2.0 {
			nodeRadius = 2.0
		}
		// Desenha os nós (estações/junções) em cada ponta da linha.
		drawNode(screen, screenX1, screenY1, nodeRadius, nodeOutlineWidth, nodeFillColor, nodeOutlineColor)
		drawNode(screen, screenX2, screenY2, nodeRadius, nodeOutlineWidth, nodeFillColor, nodeOutlineColor)
	}

	// --- Desenhar Linha em Progresso e Nós Temporários ---
	currentDrawingLengthMeters := 0.0
	// Só desenha se o usuário estiver ativamente arrastando o mouse (g.drawing = true)
	// e se o ponto inicial for válido.
	if g.drawing && !math.IsNaN(g.startX) && !math.IsNaN(g.startY) {
		// Converte posição atual do cursor para mundo (para calcular comprimento).
		currentWorldX, currentWorldY := g.screenToWorld(cursorX, cursorY)
		// Calcula o comprimento em metros da linha sendo desenhada.
		currentDrawingLengthMeters = calculateLengthMeters(g.startX, g.startY, currentWorldX, currentWorldY)
		// Converte o ponto inicial (mundo) para tela.
		startScreenX, startScreenY := g.worldToScreen(g.startX, g.startY)

		// Desenha a linha da posição inicial (tela) até a posição atual do cursor (tela).
		drawThickLine(screen, g.whitePixel, startScreenX, startScreenY, float32(cursorX), float32(cursorY), float32(g.thickness), g.currentColor, "drawing-live")

		// Calcula o raio dos nós temporários.
		nodeRadius := float32(g.thickness*nodeRadiusFactor / 2.0)
		if nodeRadius < 2.0 {
			nodeRadius = 2.0
		}
		// Define cores para os nós temporários.
		nodeFillColor := g.currentColor
		nodeOutlineColor := color.White
		// Desenha os nós temporários.
		drawNode(screen, startScreenX, startScreenY, nodeRadius, nodeOutlineWidth, nodeFillColor, nodeOutlineColor)
		drawNode(screen, float32(cursorX), float32(cursorY), nodeRadius, nodeOutlineWidth, nodeFillColor, nodeOutlineColor)
	}

	// --- Desenhar Popup ---
	// Só desenha se o popup estiver ativo.
	if g.popupVisible {
		// Calcula a posição ajustada para o popup caber na tela.
		drawPopupX, drawPopupY := g.calculatePopupDrawPosition()
		// Calcula a altura necessária para desenhar o fundo do popup.
		popupDrawHeight := 0
		if len(g.popupOptions) > 0 {
			maxYRel := 0 // Encontra o Y relativo máximo das opções.
			for _, opt := range g.popupOptions {
				relY := opt.Rect.Max.Y - g.popupY
				if relY > maxYRel {
					maxYRel = relY
				}
			}
			popupDrawHeight = maxYRel + popupPadding // Altura é o Y máximo + padding.
		}

		// Desenha o fundo retangular semi-transparente do popup.
		if popupDrawHeight > 0 {
			vector.DrawFilledRect(screen, float32(drawPopupX), float32(drawPopupY), float32(popupWidth), float32(popupDrawHeight), color.RGBA{50, 50, 50, 220}, false)
		}

		// Calcula o deslocamento entre a posição original e a de desenho.
		offsetX := drawPopupX - g.popupX
		offsetY := drawPopupY - g.popupY
		// Itera pelas opções para desenhá-las.
		for _, option := range g.popupOptions {
			// Ajusta o retângulo da opção para a posição de desenho final.
			optionDrawRect := option.Rect.Add(image.Pt(offsetX, offsetY))

			// Se a opção tem uma cor, desenha o quadrado colorido com borda.
			if option.Color != nil {
				vector.DrawFilledRect(screen, float32(optionDrawRect.Min.X), float32(optionDrawRect.Min.Y), float32(optionDrawRect.Dx()), float32(optionDrawRect.Dy()), *option.Color, false)
				vector.StrokeRect(screen, float32(optionDrawRect.Min.X), float32(optionDrawRect.Min.Y), float32(optionDrawRect.Dx()), float32(optionDrawRect.Dy()), 1, color.White, false)
			}
			// Se a opção tem um label, desenha o texto centralizado.
			if option.Label != "" {
				textBounds := text.BoundString(basicfont.Face7x13, option.Label)
				textX := optionDrawRect.Min.X + (optionDrawRect.Dx()-textBounds.Dx())/2
				textY := optionDrawRect.Min.Y + (optionDrawRect.Dy()+textBounds.Dy())/2 - 2 // Ajuste vertical empírico.
				text.Draw(screen, option.Label, basicfont.Face7x13, textX, textY, color.White)
			}
		}
	}

	// --- Desenhar Tooltip ---
	// Só desenha se houver um trilho sob o mouse, E o popup não estiver visível, E não estiver desenhando.
	if g.hoveredTrackIndex != -1 && g.hoveredTrackIndex < len(g.tracks) && !g.popupVisible && !g.drawing {
		hoveredTrack := g.tracks[g.hoveredTrackIndex]
		// Formata o texto do tooltip (metragem com 0 casas decimais).
		tooltipText := fmt.Sprintf("%.0f m", hoveredTrack.LengthMeters)
		// Calcula o tamanho que o texto ocupará.
		textBounds := text.BoundString(basicfont.Face7x13, tooltipText)

		// Calcula as dimensões do fundo do tooltip.
		tooltipW := textBounds.Dx() + tooltipPadding*2
		tooltipH := 13 + tooltipPadding*2 // Altura da fonte é 13.

		// Posição inicial do tooltip (abaixo e à direita do cursor).
		tooltipX := cursorX + 10
		tooltipY := cursorY + 15

		// Ajusta a posição para garantir que o tooltip caiba na tela.
		if tooltipX+tooltipW > g.screenWidth {
			tooltipX = g.screenWidth - tooltipW
		}
		if tooltipY+tooltipH > g.screenHeight {
			tooltipY = g.screenHeight - tooltipH
		}
		if tooltipX < 0 {
			tooltipX = 0
		}
		if tooltipY < 0 {
			tooltipY = 0
		}

		// Desenha o fundo semi-transparente do tooltip.
		vector.DrawFilledRect(screen, float32(tooltipX), float32(tooltipY), float32(tooltipW), float32(tooltipH), color.RGBA{30, 30, 30, 200}, false)
		// Desenha o texto do tooltip. Ajuste Y para centralizar verticalmente.
		text.Draw(screen, tooltipText, basicfont.Face7x13, tooltipX+tooltipPadding, tooltipY+tooltipPadding+10, color.White)
	}

	// --- Info na Tela (DebugPrint) ---
	// Monta as strings de ajuda.
	trilhoColorHelp := "Trilho [1:R 2:B 3:Y 4:G]"
	bgColorHelp := "Fundo [F1-F4]"
	scrollHelp := "Scroll [<-/->]"
	saveLoadHelp := "Salvar [S] | Carregar [L]"
	// Formata a string principal de status.
	statusText := fmt.Sprintf("Comp. Total: %.0f m | Cam Offset: %.0f | Escala: 1px=%.0fm\n%s | %s | +/-: Esp (%.0fpx)\nArrastar | %s | %s | C: Limpar | ESC: Sair",
		totalLengthMeters, g.cameraOffsetX, 1.0/pixelsPerMeter,
		trilhoColorHelp, bgColorHelp, g.thickness,
		scrollHelp, saveLoadHelp)
	// Adiciona o comprimento da linha atual, se estiver desenhando.
	if g.drawing && currentDrawingLengthMeters > 0 && !math.IsNaN(currentDrawingLengthMeters) {
		statusText += fmt.Sprintf(" | Atual: %.0f m", currentDrawingLengthMeters)
	}
	// Desenha o texto de status/ajuda na tela (deve ser a última coisa desenhada para ficar por cima).
	ebitenutil.DebugPrint(screen, statusText)
}

// Layout define o tamanho lógico da tela do jogo.
// É chamado quando a janela é criada ou redimensionada.
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	// Atualiza as dimensões armazenadas no estado do jogo.
	// Isso é importante para que a lógica que impede o popup/tooltip
	// de sair da tela use as dimensões corretas após redimensionar.
	g.screenWidth = outsideWidth
	g.screenHeight = outsideHeight
	// Retorna as dimensões para o Ebiten usar como tamanho lógico.
	return g.screenWidth, g.screenHeight
}

// drawThickLine desenha uma linha com espessura usando a técnica de triângulos.
// Recebe coordenadas da TELA.
func drawThickLine(screen *ebiten.Image, whitePixel *ebiten.Image, x1, y1, x2, y2, thickness float32, clr color.Color, id string) {
	// Verificações iniciais.
	if screen == nil || whitePixel == nil {
		log.Printf("ERRO (%s): screen/whitePixel nil em drawThickLine", id)
		return
	}
	if thickness < 1 { // Garante espessura mínima.
		thickness = 1
	}

	// Calcula vetor e comprimento ao quadrado.
	dx := x2 - x1
	dy := y2 - y1
	lengthSq := dx*dx + dy*dy
	// Se o comprimento for quase zero, não desenha nada.
	if lengthSq < 0.01 {
		return
	}
	// Calcula comprimento real.
	length := float32(math.Sqrt(float64(lengthSq)))
	// Calcula vetor normalizado (direção).
	nx := dx / length
	ny := dy / length
	// Verifica se o vetor normalizado é válido (não NaN/Inf).
	if math.IsNaN(float64(nx)) || math.IsNaN(float64(ny)) || math.IsInf(float64(nx), 0) || math.IsInf(float64(ny), 0) {
		log.Printf("ERRO (%s): NaN/Inf no vetor normalizado em drawThickLine", id)
		return
	}

	// Calcula vetor perpendicular para a espessura.
	halfThick := thickness / 2.0
	px := -ny * halfThick
	py := nx * halfThick

	// Calcula os 4 vértices do retângulo que representa a linha.
	v1x, v1y := x1+px, y1+py
	v2x, v2y := x1-px, y1-py
	v3x, v3y := x2-px, y2-py
	v4x, v4y := x2+px, y2+py

	// Verifica se as coordenadas dos vértices são válidas.
	coords := []float32{v1x, v1y, v2x, v2y, v3x, v3y, v4x, v4y}
	for i, val := range coords {
		if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
			log.Printf("ERRO (%s): NaN/Inf na coordenada final do vértice [%d] em drawThickLine", id, i)
			return
		}
	}

	// Converte a cor para componentes RGBA float32 (0.0 a 1.0).
	// Renomeia 'g' para 'gVal' para evitar conflito com o nome do pacote 'g'.
	r, gVal, b, a := clr.RGBA()
	colorR, colorG, colorB, colorA := float32(r)/65535.0, float32(gVal)/65535.0, float32(b)/65535.0, float32(a)/65535.0

	// Define os vértices para Ebiten.
	// Usa SrcX=0, SrcY=0 porque estamos passando a textura whitePixel (1x1).
	vertices := []ebiten.Vertex{
		{DstX: v1x, DstY: v1y, SrcX: 0, SrcY: 0, ColorR: colorR, ColorG: colorG, ColorB: colorB, ColorA: colorA},
		{DstX: v2x, DstY: v2y, SrcX: 0, SrcY: 0, ColorR: colorR, ColorG: colorG, ColorB: colorB, ColorA: colorA},
		{DstX: v3x, DstY: v3y, SrcX: 0, SrcY: 0, ColorR: colorR, ColorG: colorG, ColorB: colorB, ColorA: colorA},
		{DstX: v4x, DstY: v4y, SrcX: 0, SrcY: 0, ColorR: colorR, ColorG: colorG, ColorB: colorB, ColorA: colorA},
	}
	// Índices que definem os dois triângulos que formam o retângulo.
	indices := []uint16{0, 1, 2, 0, 2, 3}
	// Opções de desenho (habilita AntiAliasing para suavização).
	op := &ebiten.DrawTrianglesOptions{AntiAlias: true}
	// Desenha os triângulos usando a textura whitePixel.
	screen.DrawTriangles(vertices, indices, whitePixel, op)
}

// --- Função Principal ---

// main é o ponto de entrada da aplicação.
func main() {
	// Cria a instância principal do jogo.
	gameInstance := NewGame()
	// Configura o tamanho inicial da janela.
	ebiten.SetWindowSize(gameInstance.screenWidth, gameInstance.screenHeight)
	// Define o título da janela.
	ebiten.SetWindowTitle("Malha Ferroviária Interativa (v7.3 - Correct Formatting)")
	// Permite que a janela seja redimensionada pelo usuário.
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	log.Println("Iniciando loop principal do Ebitengine...")
	// Executa o jogo. Esta função bloqueia até o jogo terminar (erro ou ebiten.Termination).
	if err := ebiten.RunGame(gameInstance); err != nil {
		// Verifica se o erro é diferente de um término normal.
		if err != ebiten.Termination {
			log.Printf("Erro fatal durante a execução do jogo: %v", err) // Loga erros inesperados.
		} else {
			log.Println("Jogo terminado normalmente pelo usuário.")
		}
	}
	log.Println("==== Programa encerrado ====") // Mensagem final no log.
}