import AppKit
import Darwin
import Metal

struct ResourceSample: Codable {
    let elapsed: Double
    let cpuSeconds: Double
    let rssBytes: UInt64
    let footprintBytes: UInt64
    let peakRSSBytes: UInt64
    let defaultHeapInUseBytes: UInt64
    let defaultHeapAllocatedBytes: UInt64
    let metalAllocatedBytes: UInt64?
    @MainActor static func take(start: Date, device: (any MTLDevice)?) -> Self {
        var usage = rusage(); getrusage(RUSAGE_SELF, &usage)
        var info = task_vm_info_data_t()
        var count = mach_msg_type_number_t(MemoryLayout<task_vm_info_data_t>.size / MemoryLayout<integer_t>.size)
        let result = withUnsafeMutablePointer(to: &info) { pointer in
            pointer.withMemoryRebound(to: integer_t.self, capacity: Int(count)) {
                task_info(mach_task_self_, task_flavor_t(TASK_VM_INFO), $0, &count)
            }
        }
        var heap = malloc_statistics_t()
        malloc_zone_statistics(malloc_default_zone(), &heap)
        return Self(elapsed: Date().timeIntervalSince(start),
                    cpuSeconds: Double(usage.ru_utime.tv_sec + usage.ru_stime.tv_sec) + Double(usage.ru_utime.tv_usec + usage.ru_stime.tv_usec) / 1e6,
                    rssBytes: result == KERN_SUCCESS ? info.resident_size : 0,
                    footprintBytes: result == KERN_SUCCESS ? info.phys_footprint : 0,
                    peakRSSBytes: UInt64(usage.ru_maxrss), defaultHeapInUseBytes: UInt64(heap.size_in_use),
                    defaultHeapAllocatedBytes: UInt64(heap.size_allocated), metalAllocatedBytes: device.map { UInt64($0.currentAllocatedSize) })
    }
}

@MainActor final class LifetimeProbe {
    private var objects: [String: [WeakObject]] = [:]
    private(set) var created: [String: Int] = [:]
    func track(_ object: AnyObject, kind: String) {
        objects[kind, default: []].removeAll { $0.value == nil }
        objects[kind, default: []].append(WeakObject(object))
        created[kind, default: 0] += 1
    }
    var live: [String: Int] { objects.mapValues { $0.filter { $0.value != nil }.count } }
}

private final class WeakObject {
    weak var value: AnyObject?
    init(_ object: AnyObject) { value = object }
}
