export namespace main {
	
	export class EncoderStats {
	    frames_encoded: number;
	    frames_dropped: number;
	    current_fps: number;
	    average_bitrate: number;
	    last_frame_time: number;
	    pipeline_state: string;
	
	    static createFrom(source: any = {}) {
	        return new EncoderStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.frames_encoded = source["frames_encoded"];
	        this.frames_dropped = source["frames_dropped"];
	        this.current_fps = source["current_fps"];
	        this.average_bitrate = source["average_bitrate"];
	        this.last_frame_time = source["last_frame_time"];
	        this.pipeline_state = source["pipeline_state"];
	    }
	}
	export class VMConfig {
	    name: string;
	    cpus: number;
	    memory_mb: number;
	    disk_path: string;
	    vsock_cid: number;
	    ssh_port: number;
	    api_port: number;
	    video_port: number;
	    qmp_port: number;
	
	    static createFrom(source: any = {}) {
	        return new VMConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.cpus = source["cpus"];
	        this.memory_mb = source["memory_mb"];
	        this.disk_path = source["disk_path"];
	        this.vsock_cid = source["vsock_cid"];
	        this.ssh_port = source["ssh_port"];
	        this.api_port = source["api_port"];
	        this.video_port = source["video_port"];
	        this.qmp_port = source["qmp_port"];
	    }
	}
	export class VMStatus {
	    state: string;
	    cpu_percent: number;
	    memory_used: number;
	    uptime: number;
	    sessions: number;
	    error_msg?: string;
	    api_ready: boolean;
	    video_ready: boolean;
	
	    static createFrom(source: any = {}) {
	        return new VMStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.cpu_percent = source["cpu_percent"];
	        this.memory_used = source["memory_used"];
	        this.uptime = source["uptime"];
	        this.sessions = source["sessions"];
	        this.error_msg = source["error_msg"];
	        this.api_ready = source["api_ready"];
	        this.video_ready = source["video_ready"];
	    }
	}

}

