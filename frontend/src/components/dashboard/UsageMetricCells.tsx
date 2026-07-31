import { TableCell, Typography } from '@mui/material';
import { formatNumber, getCacheHitRate, getTotalTokens } from './chartStyles';
import {
    defaultUsageMetricLabels,
    getUsageMetricColumns,
} from './usageMetricColumns';
import type {
    UsageMetricLabels,
    UsageMetricOptions,
    UsageMetricSource,
} from './usageMetricColumns';

export function UsageMetricHeaderCells({
    labels = defaultUsageMetricLabels,
    showTotal = false,
    showCacheWrite = false,
}: UsageMetricOptions & { labels?: UsageMetricLabels }) {
    return getUsageMetricColumns({ showTotal, showCacheWrite }, labels).map((column) => (
        <TableCell key={column.key} align="right">
            {column.label}
        </TableCell>
    ));
}

export function UsageMetricValueCells({
    usage,
    showTotal = false,
    showCacheWrite = false,
    errorDigits = 1,
    cacheHitDigits = 1,
    requestFormatter = formatNumber,
}: UsageMetricOptions & {
    usage: UsageMetricSource;
    errorDigits?: number;
    cacheHitDigits?: number;
    requestFormatter?: (value: number) => string;
}) {
    const cacheRead = usage.cache_read_tokens || 0;
    const input = usage.total_input_tokens || 0;
    const errorRate = usage.error_rate || 0;

    return (
        <>
            <TableCell align="right">{requestFormatter(usage.request_count)}</TableCell>
            {showTotal && (
                <TableCell align="right" sx={{ fontWeight: 600 }}>
                    {formatNumber(getTotalTokens(usage))}
                </TableCell>
            )}
            <TableCell align="right">{formatNumber(cacheRead)}</TableCell>
            {showCacheWrite && (
                <TableCell align="right">{formatNumber(usage.cache_write_tokens || 0)}</TableCell>
            )}
            <TableCell align="right">{getCacheHitRate(cacheRead, input).toFixed(cacheHitDigits)}%</TableCell>
            <TableCell align="right">{formatNumber(input)}</TableCell>
            <TableCell align="right">{formatNumber(usage.total_output_tokens || 0)}</TableCell>
            <TableCell align="right">
                <Typography
                    variant="body2"
                    sx={{ color: errorRate > 0.05 ? 'error.main' : 'text.secondary' }}
                >
                    {(errorRate * 100).toFixed(errorDigits)}%
                </Typography>
            </TableCell>
        </>
    );
}
