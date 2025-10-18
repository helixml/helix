export class ComponentEvent extends Event {
    constructor(type, component) {
        super(type);
        this.component = component;
    }
}
