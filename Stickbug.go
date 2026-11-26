package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strings"
	"time"
)

const width = 90
const height = 30
const totalFrames = 140 // 140 frames for maximally smooth camera motion

const green = "\033[92m"
const brightGreen = "\033[1;92m"
const yellow = "\033[93m"
const cyan = "\033[96m"
const reset = "\033[0m"

// radians converts degrees to radians the same way Python's math.radians does
// (multiplication by pi/180).
func radians(deg float64) float64 {
	return deg * (math.Pi / 180)
}

// smoothstep: smooth acceleration at the start and deceleration at the end (no abrupt jerks).
func smoothstep(t float64) float64 {
	t = math.Max(0.0, math.Min(1.0, t))
	return t * t * (3.0 - 2.0*t)
}

// rotatePoint: rotates a 3D point around the Y axis (view angle) and the X axis (camera height).
func rotatePoint(x, y, z, yaw, pitch float64) (float64, float64, float64) {
	x1 := x*math.Cos(yaw) - z*math.Sin(yaw)
	z1 := x*math.Sin(yaw) + z*math.Cos(yaw)
	y2 := y*math.Cos(pitch) - z1*math.Sin(pitch)
	z2 := y*math.Sin(pitch) + z1*math.Cos(pitch)
	return x1, y2, z2
}

// drawLine3D: draws a 3D line between two points, taking perspective and depth into account.
func drawLine3D(screen [][]byte, zbuffer [][]float64, p1, p2 [3]float64, char byte, yaw, pitch, zoom float64) {
	steps := 32
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := p1[0] + (p2[0]-p1[0])*t
		y := p1[1] + (p2[1]-p1[1])*t
		z := p1[2] + (p2[2]-p1[2])*t

		rx, ry, rz := rotatePoint(x, y, z, yaw, pitch)
		rz += zoom

		if rz > 0.5 {
			sx := int((rx/rz)*34 + float64(width/2))
			sy := int((ry/rz)*17 + float64(height/2))

			if sx >= 0 && sx < width && sy >= 0 && sy < height {
				if rz < zbuffer[sy][sx] {
					zbuffer[sy][sx] = rz
					screen[sy][sx] = char
				}
			}
		}
	}
}

func renderFrame(frameIdx int) string {
	screen := make([][]byte, height)
	zbuffer := make([][]float64, height)
	for r := 0; r < height; r++ {
		screen[r] = make([]byte, width)
		zbuffer[r] = make([]float64, width)
		for c := 0; c < width; c++ {
			screen[r][c] = ' '
			zbuffer[r][c] = 999.0
		}
	}

	progress := float64(frameIdx) / float64(totalFrames)
	angleCycle := progress * 2 * math.Pi

	// --- SMOOTH CAMERA FLYOVER (SIDE -> BACK -> BACK TO START) ---
	var blend float64
	var camLabel string
	if frameIdx < 30 {
		// 1. Calm side view (frames 1-30)
		blend = 0.0
		camLabel = "SIDE VIEW (ЗБОКУ)"
	} else if frameIdx < 75 {
		// 2. Smooth camera move from the side to behind (frames 31-75)
		blend = smoothstep(float64(frameIdx-30) / 45.0)
		camLabel = "MOVING TO BACK... (ПЛАВНИЙ ОБЛІТ)"
	} else if frameIdx < 105 {
		// 3. Back view (frames 76-105)
		blend = 1.0
		camLabel = "BACK VIEW (ЗЗАДУ)"
	} else {
		// 4. Smooth return of the camera to the side view for looping (frames 106-140)
		blend = 1.0 - smoothstep(float64(frameIdx-105)/35.0)
		camLabel = "RETURNING TO SIDE... (ПОВЕРНЕННЯ)"
	}

	// Smooth angle change from 80° (side) to 180° (back)
	yaw := radians(80.0 + blend*100.0)
	pitch := radians(15.0 + blend*4.0)
	zoom := 4.1 - blend*0.25

	// --- PENDULUM SWAY (STICK BUG SWAY) ---
	// 12 full swings left-right over 140 frames
	swayPhase := angleCycle * 12

	swayX := math.Sin(swayPhase) * 0.48
	arcY := -math.Abs(math.Sin(swayPhase))*0.16 + 0.08
	headExtraX := math.Sin(swayPhase-0.35) * 0.18

	// Body coordinates (head on +Z, tail on -Z)
	neck := [3]float64{swayX, -0.32 + arcY, 0.75}
	midBody := [3]float64{swayX * 0.85, -0.22 + arcY, 0.0}
	tail := [3]float64{swayX * 0.65, -0.12 + arcY, -1.15}
	head := [3]float64{swayX + headExtraX, -0.50 + arcY, 1.05}

	// Draw body and head
	drawLine3D(screen, zbuffer, tail, midBody, '#', yaw, pitch, zoom)
	drawLine3D(screen, zbuffer, midBody, neck, '#', yaw, pitch, zoom)
	drawLine3D(screen, zbuffer, neck, head, '@', yaw, pitch, zoom)

	// Antennae on the head
	antSway := math.Sin(swayPhase-0.7) * 0.15
	antL := [3]float64{head[0] - 0.25 + antSway, head[1] - 0.35, head[2] + 0.35}
	antR := [3]float64{head[0] + 0.25 + antSway, head[1] - 0.35, head[2] + 0.35}
	drawLine3D(screen, zbuffer, head, antL, '/', yaw, pitch, zoom)
	drawLine3D(screen, zbuffer, head, antR, '\\', yaw, pitch, zoom)

	// --- 6 LEGS (FEET ON THE GROUND, KNEES BEND DURING SWAY) ---
	groundY := 0.85

	type leg struct {
		attach   [3]float64
		kneeBase [3]float64
		footBase [3]float64
	}

	legs := []leg{
		{neck, [3]float64{-0.75, -0.1, 0.95}, [3]float64{-1.05, groundY, 1.15}},
		{neck, [3]float64{0.75, -0.1, 0.95}, [3]float64{1.05, groundY, 1.15}},
		{midBody, [3]float64{-0.95, -0.15, 0.0}, [3]float64{-1.25, groundY, 0.0}},
		{midBody, [3]float64{0.95, -0.15, 0.0}, [3]float64{1.25, groundY, 0.0}},
		{tail, [3]float64{-0.85, -0.1, -0.95}, [3]float64{-1.15, groundY, -1.25}},
		{tail, [3]float64{0.85, -0.1, -0.95}, [3]float64{1.15, groundY, -1.25}},
	}

	for _, l := range legs {
		knee := [3]float64{
			l.kneeBase[0] + swayX*0.45,
			l.kneeBase[1] + arcY*0.5,
			l.kneeBase[2],
		}
		drawLine3D(screen, zbuffer, l.attach, knee, '+', yaw, pitch, zoom)
		drawLine3D(screen, zbuffer, knee, l.footBase, '|', yaw, pitch, zoom)
	}

	// HUD in the bottom row
	camInfo := fmt.Sprintf("FRAME: %03d/%d | %s", frameIdx+1, totalFrames, camLabel)
	lines := make([]string, height)
	for i, row := range screen {
		lines[i] = string(row)
	}
	titleColor := yellow
	if (frameIdx/7)%2 != 0 {
		titleColor = brightGreen
	}
	footer := titleColor + "   [ GET STICK BUGGED LOL! ]   " + cyan + camInfo + reset

	return "\033[H" + green + strings.Join(lines, "\n") + "\n" + footer
}

func main() {
	fmt.Print("\033[2J\033[?25l")

	framesCache := make([]string, totalFrames)
	for i := 0; i < totalFrames; i++ {
		framesCache[i] = renderFrame(i)
	}

	// Ctrl-C (SIGINT) cancels the context, mirroring Python's KeyboardInterrupt.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	idx := 0
	for {
		fmt.Print(framesCache[idx])
		idx = (idx + 1) % totalFrames

		select {
		case <-ctx.Done():
			fmt.Print(reset + "\033[?25h\n")
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}
