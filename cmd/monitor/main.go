package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"proxy-v6/pkg/models"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

type model struct {
	coordinatorURL string
	nodes          []models.NodeInfo
	stats          map[string]interface{}
	table          table.Model
	lastUpdate     time.Time
	err            error
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tickCmd(), m.fetchData())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			return m, m.fetchData()
		}
		
	case tickMsg:
		return m, tea.Batch(tickCmd(), m.fetchData())
		
	case nodesMsg:
		m.nodes = msg.nodes
		m.stats = msg.stats
		m.lastUpdate = time.Now()
		m.updateTable()
		
	case errMsg:
		m.err = msg.err
	}
	
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m model) View() string {
	var s string
	
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		MarginBottom(1)
	
	s += headerStyle.Render("IPv6 Proxy Monitor") + "\n"
	s += fmt.Sprintf("Last Update: %s\n\n", m.lastUpdate.Format("15:04:05"))
	
	if m.stats != nil {
		statsStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)
		
		statsText := fmt.Sprintf(
			"Total Nodes: %v\nTotal Proxies: %v\nHealthy Proxies: %v",
			m.stats["total_nodes"],
			m.stats["total_proxies"],
			m.stats["healthy_proxies"],
		)
		s += statsStyle.Render(statsText) + "\n\n"
	}
	
	s += m.table.View() + "\n\n"
	
	if m.err != nil {
		errStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))
		s += errStyle.Render(fmt.Sprintf("Error: %v", m.err)) + "\n"
	}
	
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))
	s += helpStyle.Render("Press 'q' to quit, 'r' to refresh")
	
	return s
}

func (m *model) updateTable() {
	columns := []table.Column{
		{Title: "Node ID", Width: 20},
		{Title: "Hostname", Width: 20},
		{Title: "Proxies", Width: 10},
		{Title: "Running", Width: 10},
		{Title: "Last Update", Width: 20},
	}
	
	var rows []table.Row
	for _, node := range m.nodes {
		runningCount := 0
		for _, proxy := range node.Proxies {
			if proxy.Status == models.ProxyStatusRunning {
				runningCount++
			}
		}
		
		rows = append(rows, table.Row{
			node.NodeID,
			node.Hostname,
			fmt.Sprintf("%d", len(node.Proxies)),
			fmt.Sprintf("%d", runningCount),
			node.UpdatedAt.Format("15:04:05"),
		})
	}
	
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)
	
	m.table = t
}

type nodesMsg struct {
	nodes []models.NodeInfo
	stats map[string]interface{}
}

type errMsg struct {
	err error
}

func (m model) fetchData() tea.Cmd {
	return func() tea.Msg {
		client := &http.Client{Timeout: 5 * time.Second}
		
		resp, err := client.Get(fmt.Sprintf("%s/api/nodes", m.coordinatorURL))
		if err != nil {
			return errMsg{err: err}
		}
		defer resp.Body.Close()
		
		var nodes []models.NodeInfo
		if err := json.NewDecoder(resp.Body).Decode(&nodes); err != nil {
			return errMsg{err: err}
		}
		
		resp, err = client.Get(fmt.Sprintf("%s/api/stats", m.coordinatorURL))
		if err != nil {
			return errMsg{err: err}
		}
		defer resp.Body.Close()
		
		var stats map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
			return errMsg{err: err}
		}
		
		return nodesMsg{nodes: nodes, stats: stats}
	}
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "monitor",
		Short: "TUI monitor for IPv6 proxy system",
		Run: func(cmd *cobra.Command, args []string) {
			coordinatorURL, _ := cmd.Flags().GetString("coordinator")
			
			m := model{
				coordinatorURL: coordinatorURL,
				lastUpdate:     time.Now(),
			}
			m.updateTable()
			
			p := tea.NewProgram(m, tea.WithAltScreen())
			if _, err := p.Run(); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		},
	}
	
	rootCmd.Flags().StringP("coordinator", "c", "http://localhost:8081", "Coordinator URL")
	
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
	}
}