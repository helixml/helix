// Basic AGS configuration for illogical-impulse fallback
// This provides a minimal desktop shell when quickshell is not available

const hyprland = await Service.import("hyprland")
const battery = await Service.import("battery")
const systemtray = await Service.import("systemtray")

// Top bar with basic functionality
const Bar = (monitor = 0) => Widget.Window({
    name: `bar-${monitor}`,
    class_name: "bar",
    monitor,
    anchor: ["top", "left", "right"],
    exclusivity: "exclusive",
    child: Widget.CenterBox({
        start_widget: Widget.Box({
            children: [
                // Workspace indicator
                Widget.Label({
                    label: hyprland.active.workspace.bind("id").as(id => `${id}`),
                    class_name: "workspace"
                }),
            ],
        }),
        center_widget: Widget.Box({
            children: [
                // Window title
                Widget.Label({
                    label: hyprland.active.client.bind("title").as(title => title || "Desktop"),
                    class_name: "window-title"
                }),
            ],
        }),
        end_widget: Widget.Box({
            hpack: "end",
            children: [
                // System tray
                Widget.Box({
                    children: systemtray.bind("items").as(items => items.map(item =>
                        Widget.Button({
                            child: Widget.Icon({ icon: item.bind("icon") }),
                            on_primary_click: (_, event) => item.activate(event),
                            on_secondary_click: (_, event) => item.openMenu(event),
                            tooltip_markup: item.bind("tooltip_markup"),
                        })
                    )),
                }),
                // Battery indicator
                Widget.Label({
                    label: battery.bind("percent").as(p => `${p}%`),
                    visible: battery.bind("available"),
                    class_name: "battery"
                }),
                // Clock
                Widget.Label({
                    label: new Date().toLocaleTimeString(),
                    class_name: "clock"
                }),
            ],
        }),
    }),
})

// Simple CSS for styling
const scss = `
.bar {
    background-color: rgba(30, 30, 46, 0.9);
    color: #cdd6f4;
    font-family: 'JetBrains Mono', monospace;
    font-size: 14px;
    padding: 0 16px;
}

.workspace {
    background-color: #89b4fa;
    color: #1e1e2e;
    padding: 4px 8px;
    border-radius: 4px;
    margin-right: 8px;
}

.window-title {
    font-weight: bold;
}

.battery, .clock {
    margin-left: 8px;
    padding: 4px;
}
`

App.config({
    style: scss,
    windows: [Bar()],
})