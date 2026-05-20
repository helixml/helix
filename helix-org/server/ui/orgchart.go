package ui

import (
	"fmt"
	"html"
	"strings"

	"github.com/helixml/helix-org/domain"
)

// renderOrgChart returns an SVG fragment laying out positions as a
// top-down tree, with workers attached as small badges below their
// position node. Returns an empty string when there are no
// positions — the template hides the chart section in that case.
//
// Layout: each node is a 200×64 rectangle with rounded corners,
// connected to its parent by a vertical-then-horizontal edge.
// Subtree widths are computed bottom-up so siblings fan out without
// overlapping.
func renderOrgChart(positions []domain.Position, workers []domain.Worker) string {
	if len(positions) == 0 {
		return ""
	}

	// Group workers by the positions they fill.
	byPos := make(map[domain.PositionID][]domain.Worker)
	for _, w := range workers {
		for _, pid := range w.Positions() {
			byPos[pid] = append(byPos[pid], w)
		}
	}

	// Build tree from positions. Roots are positions with empty
	// ParentID (or whose parent isn't in the slice — defensive).
	idx := make(map[domain.PositionID]domain.Position, len(positions))
	for _, p := range positions {
		idx[p.ID] = p
	}
	type node struct {
		pos      domain.Position
		children []*node
		w        float64 // subtree width
		x, y     float64 // top-left after layout
	}
	nodes := make(map[domain.PositionID]*node, len(positions))
	for _, p := range positions {
		nodes[p.ID] = &node{pos: p}
	}
	var roots []*node
	for _, p := range positions {
		if p.ParentID == nil {
			roots = append(roots, nodes[p.ID])
			continue
		}
		if parent, ok := nodes[*p.ParentID]; ok {
			parent.children = append(parent.children, nodes[p.ID])
		} else {
			roots = append(roots, nodes[p.ID])
		}
	}

	const (
		nodeW   = 200.0
		nodeH   = 64.0
		gapX    = 32.0
		gapY    = 56.0
		padding = 16.0
	)

	var measure func(n *node) float64
	measure = func(n *node) float64 {
		if len(n.children) == 0 {
			n.w = nodeW
			return nodeW
		}
		total := 0.0
		for i, c := range n.children {
			total += measure(c)
			if i > 0 {
				total += gapX
			}
		}
		if total < nodeW {
			total = nodeW
		}
		n.w = total
		return total
	}

	totalW := 0.0
	for i, r := range roots {
		totalW += measure(r)
		if i > 0 {
			totalW += gapX
		}
	}

	var place func(n *node, x, y float64)
	place = func(n *node, x, y float64) {
		n.x = x + (n.w-nodeW)/2
		n.y = y
		cx := x
		for _, c := range n.children {
			place(c, cx, y+nodeH+gapY)
			cx += c.w + gapX
		}
	}
	rootX := padding
	for _, r := range roots {
		place(r, rootX, padding)
		rootX += r.w + gapX
	}

	// Compute total height = max y reached + nodeH + padding.
	maxY := 0.0
	var walk func(n *node)
	walk = func(n *node) {
		if n.y+nodeH > maxY {
			maxY = n.y + nodeH
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	for _, r := range roots {
		walk(r)
	}
	width := totalW + 2*padding
	height := maxY + padding

	var b strings.Builder
	// width/height attributes pin the SVG to its natural pixel size so
	// a one-node chart doesn't get stretched to fill the container.
	// max-width:100% still caps it if the chart grows wider than the
	// page; the parent has overflow-x-auto for horizontal scroll.
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f" width="%.0f" height="%.0f" style="max-width: 100%%; height: auto;">`, width, height, width, height)
	// Edges first so nodes paint over them.
	var drawEdges func(n *node)
	drawEdges = func(n *node) {
		px := n.x + nodeW/2
		py := n.y + nodeH
		for _, c := range n.children {
			cx := c.x + nodeW/2
			cy := c.y
			midY := (py + cy) / 2
			fmt.Fprintf(&b,
				`<path d="M %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f" fill="none" stroke="#C9C0AB" stroke-width="1.4" />`,
				px, py, px, midY, cx, midY, cx, cy,
			)
			drawEdges(c)
		}
	}
	for _, r := range roots {
		drawEdges(r)
	}
	// Nodes. Each position group and worker badge gets htmx attrs that
	// fetch /ui/org/detail and swap the result into #org-detail. We
	// emit them on the outer <g> rather than wrapping in <a> because
	// htmx 2 picks up clicks on any element with hx-* attrs and SVG
	// <a> would also need xlink:href to route correctly.
	var drawNodes func(n *node)
	drawNodes = func(n *node) {
		ws := byPos[n.pos.ID]
		title := html.EscapeString(string(n.pos.ID))
		role := html.EscapeString(string(n.pos.RoleID))
		posID := html.EscapeString(string(n.pos.ID))
		fmt.Fprintf(&b,
			`<g transform="translate(%.1f %.1f)" style="cursor: pointer;" hx-get="/ui/org/detail?pos=%s" hx-target="#org-detail" hx-swap="innerHTML">`+
				`<rect width="%.0f" height="%.0f" rx="10" ry="10" fill="#FFFFFF" stroke="#E4DED1" stroke-width="1" />`+
				`<text x="14" y="22" font-family="Geist Mono, ui-monospace, monospace" font-size="12" fill="#15140F">%s</text>`+
				`<text x="14" y="40" font-family="Geist, ui-sans-serif, sans-serif" font-size="11" fill="#8A8474">%s</text>`,
			n.x, n.y, posID, nodeW, nodeH, title, role,
		)
		// Worker badges along the bottom of the rect. Each badge is
		// itself clickable; we stop event propagation on the badge so
		// clicking it doesn't also trigger the parent position node.
		bx := 14.0
		by := 56.0
		for i, w := range ws {
			if i >= 3 { // cap to keep the node tidy
				fmt.Fprintf(&b,
					`<text x="%.1f" y="%.1f" font-family="Geist Mono, ui-monospace, monospace" font-size="10" fill="#8A8474">+%d</text>`,
					bx, by, len(ws)-3,
				)
				break
			}
			label := string(w.ID())
			if len(label) > 14 {
				label = label[:14] + "…"
			}
			labelEsc := html.EscapeString(label)
			workerID := html.EscapeString(string(w.ID()))
			padW := 6.0
			textW := float64(len(label)) * 6.0
			fmt.Fprintf(&b,
				`<g transform="translate(%.1f %.1f)" style="cursor: pointer;" `+
					`hx-get="/ui/org/detail?worker=%s" hx-target="#org-detail" hx-swap="innerHTML" `+
					`onclick="event.stopPropagation()">`+
					`<rect width="%.1f" height="16" rx="8" ry="8" fill="#3F5B43" />`+
					`<text x="%.1f" y="11.5" font-family="Geist Mono, ui-monospace, monospace" font-size="10" fill="#FBFAF5">%s</text>`+
					`</g>`,
				bx, by-12, workerID, textW+2*padW, padW, labelEsc,
			)
			bx += textW + 2*padW + 6
		}
		fmt.Fprintf(&b, `</g>`)
		for _, c := range n.children {
			drawNodes(c)
		}
	}
	for _, r := range roots {
		drawNodes(r)
	}
	b.WriteString(`</svg>`)
	return b.String()
}
