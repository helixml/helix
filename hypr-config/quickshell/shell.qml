// Basic quickshell configuration for illogical-impulse dotfiles
// This provides a minimal working desktop environment

import QtQuick 2.15
import QtQuick.Controls 2.15
import Quickshell 2.0
import Quickshell.Wayland 1.0

ShellRoot {
    Scope {
        id: scope
        
        // Panel window for taskbar/status bar
        PanelWindow {
            id: panel
            anchors {
                left: true
                right: true
                top: true
            }
            height: 40
            color: "#1e1e2e"
            
            Rectangle {
                anchors.fill: parent
                color: "#1e1e2e"
                border.color: "#cdd6f4"
                border.width: 1
                
                Row {
                    anchors.left: parent.left
                    anchors.leftMargin: 10
                    anchors.verticalCenter: parent.verticalCenter
                    spacing: 10
                    
                    Text {
                        text: "Hyprland + Quickshell"
                        color: "#cdd6f4"
                        font.pixelSize: 14
                    }
                }
                
                Row {
                    anchors.right: parent.right
                    anchors.rightMargin: 10
                    anchors.verticalCenter: parent.verticalCenter
                    spacing: 10
                    
                    Text {
                        text: Qt.formatDateTime(new Date(), "hh:mm")
                        color: "#cdd6f4"
                        font.pixelSize: 14
                        
                        Timer {
                            interval: 1000
                            running: true
                            repeat: true
                            onTriggered: parent.text = Qt.formatDateTime(new Date(), "hh:mm")
                        }
                    }
                }
            }
        }
    }
}