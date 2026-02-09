export namespace main {
	
	export class AppSettings {
	    vm_cpus: number;
	    vm_memory_mb: number;
	    ssh_port: number;
	    api_port: number;
	    video_port: number;
	    auto_start_vm: boolean;
	    vm_disk_path: string;
	
	    static createFrom(source: any = {}) {
	        return new AppSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.vm_cpus = source["vm_cpus"];
	        this.vm_memory_mb = source["vm_memory_mb"];
	        this.ssh_port = source["ssh_port"];
	        this.api_port = source["api_port"];
	        this.video_port = source["video_port"];
	        this.auto_start_vm = source["auto_start_vm"];
	        this.vm_disk_path = source["vm_disk_path"];
	    }
	}
	export class DiskUsage {
	    root_disk_total: number;
	    root_disk_used: number;
	    root_disk_free: number;
	    zfs_disk_total: number;
	    zfs_disk_used: number;
	    zfs_disk_free: number;
	    host_actual: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new DiskUsage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root_disk_total = source["root_disk_total"];
	        this.root_disk_used = source["root_disk_used"];
	        this.root_disk_free = source["root_disk_free"];
	        this.zfs_disk_total = source["zfs_disk_total"];
	        this.zfs_disk_used = source["zfs_disk_used"];
	        this.zfs_disk_free = source["zfs_disk_free"];
	        this.host_actual = source["host_actual"];
	        this.error = source["error"];
	    }
	}
	export class DisplayInfo {
	    name: string;
	    connected: boolean;
	    width: number;
	    height: number;
	
	    static createFrom(source: any = {}) {
	        return new DisplayInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.connected = source["connected"];
	        this.width = source["width"];
	        this.height = source["height"];
	    }
	}
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
	export class ScanoutStats {
	    total_connectors: number;
	    active_displays: number;
	    max_scanouts: number;
	    displays?: DisplayInfo[];
	    last_updated: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ScanoutStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total_connectors = source["total_connectors"];
	        this.active_displays = source["active_displays"];
	        this.max_scanouts = source["max_scanouts"];
	        this.displays = this.convertValues(source["displays"], DisplayInfo);
	        this.last_updated = source["last_updated"];
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TrayStatus {
	    vm_state: string;
	    session_count: number;
	    api_ready: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TrayStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.vm_state = source["vm_state"];
	        this.session_count = source["session_count"];
	        this.api_ready = source["api_ready"];
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
	export class ZFSStats {
	    pool_name: string;
	    pool_size: number;
	    pool_used: number;
	    pool_available: number;
	    dedup_ratio: number;
	    compression_ratio: number;
	    last_updated: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ZFSStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pool_name = source["pool_name"];
	        this.pool_size = source["pool_size"];
	        this.pool_used = source["pool_used"];
	        this.pool_available = source["pool_available"];
	        this.dedup_ratio = source["dedup_ratio"];
	        this.compression_ratio = source["compression_ratio"];
	        this.last_updated = source["last_updated"];
	        this.error = source["error"];
	    }
	}

}

