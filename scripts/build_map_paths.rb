#!/usr/bin/env ruby
# frozen_string_literal: true

# Pre-project the world atlas into SVG paths for the MCP map view.
#
# The MCP App view must be a single self-contained HTML document, so it
# cannot load d3-geo and topojson at runtime the way the dashboard does.
# This decodes the topology and projects it once, here, writing a compact
# {"<iso numeric>": {"n": name, "d": "<path>"}} map the view renders with
# no libraries at all.
#
#   ruby scripts/build_map_paths.rb
#
# Writes crates/kaunta/src/mcp/views/world-paths.json.

require "json"
require "pathname"

ROOT = Pathname.new(__dir__).parent
ATLAS = ROOT / "assets/data/countries-110m.json"
TARGET = ROOT / "crates/kaunta/src/mcp/views/world-paths.json"

# Equirectangular into a 720x360 viewBox: cheap, recognizable, and the
# inverse is trivial if the view ever needs to hit-test.
WIDTH = 720.0
HEIGHT = 360.0
# Antarctica is a projection artifact more than a visitor source; dropping
# it reclaims vertical space and a chunk of path data.
SKIP = ["010"].freeze
# Coastline detail finer than this is invisible at the rendered size, and
# it is most of the file.
MIN_STEP = 1.0
# Islands smaller than this never read as a shape; they only cost bytes.
MIN_EXTENT = 1.2

# Delta-decoded, quantized arcs -> absolute [lon, lat] rings.
def decode_arcs(topology)
  scale_x, scale_y = topology["transform"]["scale"]
  translate_x, translate_y = topology["transform"]["translate"]
  topology["arcs"].map do |arc|
    x = 0
    y = 0
    arc.map do |dx, dy|
      x += dx
      y += dy
      [(x * scale_x) + translate_x, (y * scale_y) + translate_y]
    end
  end
end

def ring(arcs, indexes)
  indexes.each_with_object([]) do |index, points|
    # A negative index means the arc, reversed, minus its shared point.
    segment = index.negative? ? arcs[~index].reverse : arcs[index]
    points.concat(points.empty? ? segment : segment[1..])
  end
end

def project(lon, lat)
  [((lon + 180.0) / 360.0) * WIDTH, ((90.0 - lat) / 180.0) * HEIGHT]
end

# Equirectangular draws a 180 degree wrap as a line across the whole map.
# Break the ring wherever consecutive longitudes jump more than half the
# world, so Fiji and Russia stop streaking through the Atlantic.
def split_at_antimeridian(points)
  segments = [[]]
  previous = nil
  points.each do |lon, lat|
    segments << [] if previous && (lon - previous).abs > 180.0
    segments.last << [lon, lat]
    previous = lon
  end
  segments.select { |segment| segment.length >= 3 }
end

def subpath(points)
  projected = points.map { |lon, lat| project(lon, lat) }
  xs = projected.map(&:first)
  ys = projected.map(&:last)
  return "" if (xs.max - xs.min) < MIN_EXTENT && (ys.max - ys.min) < MIN_EXTENT

  drawn = []
  projected.each do |x, y|
    pair = [x.round, y.round]
    if drawn.empty?
      drawn << pair
      next
    end
    previous = drawn.last
    gap = (pair[0] - previous[0]).abs + (pair[1] - previous[1]).abs
    drawn << pair if gap >= MIN_STEP
  end
  return "" if drawn.length < 3

  head = "M#{drawn.first[0]} #{drawn.first[1]}"
  tail = drawn[1..].map { |x, y| "L#{x} #{y}" }.join
  "#{head}#{tail}Z"
end

def path_data(rings)
  rings.flat_map { |points| split_at_antimeridian(points).map { |part| subpath(part) } }
       .reject(&:empty?)
       .join
end


topology = JSON.parse(ATLAS.read)
arcs = decode_arcs(topology)
countries = {}

topology["objects"]["countries"]["geometries"].each do |geometry|
  code = geometry["id"].to_s.rjust(3, "0")
  next unless code.match?(/\A\d{3}\z/)
  next if SKIP.include?(code)

  rings =
    case geometry["type"]
    when "Polygon" then geometry["arcs"].map { |part| ring(arcs, part) }
    when "MultiPolygon" then geometry["arcs"].flat_map { |polygon| polygon.map { |part| ring(arcs, part) } }
    else next
    end

  data = path_data(rings)
  countries[code] = { "n" => geometry.dig("properties", "name").to_s, "d" => data } unless data.empty?
end

sorted = countries.keys.sort.to_h { |code| [code, countries[code]] }
TARGET.write(JSON.generate(sorted))
puts "#{sorted.size} countries -> #{TARGET.relative_path_from(ROOT)} (#{TARGET.size / 1024} KB)"
