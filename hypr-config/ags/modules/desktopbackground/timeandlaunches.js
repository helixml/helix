const { GLib } = imports.gi;
import App from 'resource:///com/github/Aylur/ags/app.js';
import Widget from 'resource:///com/github/Aylur/ags/widget.js';
import Service from 'resource:///com/github/Aylur/ags/service.js';
import * as Utils from 'resource:///com/github/Aylur/ags/utils.js';

import Variable from 'resource:///com/github/Aylur/ags/variable.js';
const { execAsync, exec } = Utils;
const { Box, Label, Button, Revealer, EventBox } = Widget;
import { setupCursorHover } from '../.widgetutils/cursorhover.js';
import { quickLaunchItems } from './data_quicklaunches.js'

const TimeAndDate = () => Box({
    vertical: true,
    className: 'spacing-v--5',
    children: [
        Label({
            className: 'bg-time-clock',
            xalign: 0,
            label: GLib.DateTime.new_now_local().format(userOptions.time.format),
            setup: (self) => self.poll(userOptions.time.interval, label => {
                label.label = GLib.DateTime.new_now_local().format(userOptions.time.format);
            }),
        }),
        Label({
            className: 'bg-time-date',
            xalign: 0,
            label: GLib.DateTime.new_now_local().format(userOptions.time.dateFormatLong),
            setup: (self) => self.poll(userOptions.time.dateInterval, (label) => {
                label.label = GLib.DateTime.new_now_local().format(userOptions.time.dateFormatLong);
            }),
        }),
    ]
})

const QuickLaunches = () => Box({
    vertical: true,
    className: 'spacing-v-10',
    children: [
        Label({
            xalign: 0,
            className: 'bg-quicklaunch-title',
            label: 'Quick Launches',
        }),
        Box({
            hpack: 'start',
            className: 'spacing-h-5',
            children: quickLaunchItems.map((item, i) => Button({
                onClicked: () => {
                    execAsync(['bash', '-c', `${item["command"]}`]).catch(print);
                },
                className: 'bg-quicklaunch-btn',
                child: Label({
                    label: `${item["name"]}`,
                }),
                setup: (self) => {
                    setupCursorHover(self);
                }
            })),
        })
    ]
})

export default () => Box({
    hpack: 'start',
    vpack: 'end',
    vertical: true,
    className: 'bg-time-box spacing-h--10',
    children: [
        TimeAndDate(),
        // QuickLaunches(),
    ],
})

