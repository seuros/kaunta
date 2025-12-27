// Datastar for SSE-driven reactivity (auto-initializes via data-* attributes)
import '../cmd/kaunta/assets/vendor/datastar.js';

import Chart from 'chart.js/auto';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';
import * as topojson from 'topojson-client';

declare global {
    interface Window {
        Chart: typeof Chart;
        L: typeof L;
        topojson: typeof topojson;
    }
}

window.Chart = Chart;
window.L = L;
window.topojson = topojson;
