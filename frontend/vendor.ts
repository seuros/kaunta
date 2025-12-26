// Alpine.js removed - using Datastar for reactivity
import Chart from 'chart.js/auto';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';
import * as topojson from 'topojson-client';
// Datastar loaded separately as /assets/vendor/datastar.js

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
