export namespace main {
	
	export class AppSettings {
	    vm_cpus: number;
	    vm_memory_mb: number;
	    data_disk_size_gb: number;
	    ssh_port: number;
	    api_port: number;
	    expose_on_network: boolean;
	    require_auth_on_network: boolean;
	    new_users_are_admin: boolean;
	    allow_registration: boolean;
	    auto_start_vm: boolean;
	    vm_disk_path: string;
	    license_key?: string;
	    // Go type: time
	    trial_started_at?: any;
	    desktop_secret?: string;
	    console_password?: string;
	
	    static createFrom(source: any = {}) {
	        return new AppSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.vm_cpus = source["vm_cpus"];
	        this.vm_memory_mb = source["vm_memory_mb"];
	        this.data_disk_size_gb = source["data_disk_size_gb"];
	        this.ssh_port = source["ssh_port"];
	        this.api_port = source["api_port"];
	        this.expose_on_network = source["expose_on_network"];
	        this.require_auth_on_network = source["require_auth_on_network"];
	        this.new_users_are_admin = source["new_users_are_admin"];
	        this.allow_registration = source["allow_registration"];
	        this.auto_start_vm = source["auto_start_vm"];
	        this.vm_disk_path = source["vm_disk_path"];
	        this.license_key = source["license_key"];
	        this.trial_started_at = this.convertValues(source["trial_started_at"], null);
	        this.desktop_secret = source["desktop_secret"];
	        this.console_password = source["console_password"];
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
	export class DownloadProgress {
	    file: string;
	    bytes_done: number;
	    bytes_total: number;
	    percent: number;
	    speed: string;
	    eta: string;
	    status: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new DownloadProgress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file = source["file"];
	        this.bytes_done = source["bytes_done"];
	        this.bytes_total = source["bytes_total"];
	        this.percent = source["percent"];
	        this.speed = source["speed"];
	        this.eta = source["eta"];
	        this.status = source["status"];
	        this.error = source["error"];
	    }
	}
	export class LicenseStatus {
	    state: string;
	    trial_ends_at?: string;
	    licensed_to?: string;
	    expires_at?: string;
	
	    static createFrom(source: any = {}) {
	        return new LicenseStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.trial_ends_at = source["trial_ends_at"];
	        this.licensed_to = source["licensed_to"];
	        this.expires_at = source["expires_at"];
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
	export class VMConfig {
	    name: string;
	    cpus: number;
	    memory_mb: number;
	    disk_path: string;
	    vsock_cid: number;
	    ssh_port: number;
	    api_port: number;
	    qmp_port: number;
	    frame_export_port: number;
	    expose_on_network: boolean;
	
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
	        this.qmp_port = source["qmp_port"];
	        this.frame_export_port = source["frame_export_port"];
	        this.expose_on_network = source["expose_on_network"];
	    }
	}
	export class VMStatus {
	    state: string;
	    boot_stage?: string;
	    cpu_percent: number;
	    memory_used: number;
	    uptime: number;
	    sessions: number;
	    error_msg?: string;
	    api_ready: boolean;
	
	    static createFrom(source: any = {}) {
	        return new VMStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.boot_stage = source["boot_stage"];
	        this.cpu_percent = source["cpu_percent"];
	        this.memory_used = source["memory_used"];
	        this.uptime = source["uptime"];
	        this.sessions = source["sessions"];
	        this.error_msg = source["error_msg"];
	        this.api_ready = source["api_ready"];
	    }
	}
	export class ZFSDatasetStats {
	    name: string;
	    used: number;
	    referenced: number;
	    type: string;
	
	    static createFrom(source: any = {}) {
	        return new ZFSDatasetStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.used = source["used"];
	        this.referenced = source["referenced"];
	        this.type = source["type"];
	    }
	}
	export class ZFSStats {
	    pool_name: string;
	    pool_size: number;
	    pool_used: number;
	    pool_available: number;
	    dedup_ratio: number;
	    compression_ratio: number;
	    dedup_saved_bytes: number;
	    datasets: ZFSDatasetStats[];
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
	        this.dedup_saved_bytes = source["dedup_saved_bytes"];
	        this.datasets = this.convertValues(source["datasets"], ZFSDatasetStats);
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

}

