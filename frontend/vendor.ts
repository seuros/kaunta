import Alpine from 'alpinejs';
import intersect from '@alpinejs/intersect';
import Chart from 'chart.js/auto';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';
import * as topojson from 'topojson-client';

declare global {
    interface Window {
        Alpine: typeof Alpine;
        Chart: typeof Chart;
        L: typeof L;
        topojson: typeof topojson;
    }
}

window.Alpine = Alpine;
window.Chart = Chart;
window.L = L;
window.topojson = topojson;

Alpine.plugin(intersect);
window.Alpine.start();
