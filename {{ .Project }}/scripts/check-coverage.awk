NR == 1 { next }

{{ if .Computed.enable_grpc_final -}}
$1 ~ /\.pb\.go:/ { next }
{{ end -}}

{
    split($1, location, ":")
    packageName = location[1]
    sub("/[^/]+$", "", packageName)
    total[packageName] += $2
    if ($3 > 0) {
        covered[packageName] += $2
    }
}

END {
    failed = 0
    modulePackages = 0
    for (packageName in total) {
        required = 60
        if (index(packageName, "/internal/modules/") > 0) {
            required = 80
            modulePackages++
        }
        coverage = 100 * covered[packageName] / total[packageName]
        printf "%s: %.1f%% (required %.0f%%)\n", packageName, coverage, required
        if (coverage + 0.0001 < required) {
            failed = 1
        }
    }
    if (modulePackages == 0) {
        print "internal/modules: no packages, skipped"
    }
    exit failed
}
