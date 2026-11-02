import java.nio.file.*;
import java.util.*;
import javax.imageio.ImageIO;
import java.awt.image.BufferedImage;
import com.google.gson.*;
import net.minecraft.SharedConstants;
import net.minecraft.server.Bootstrap;
import net.minecraft.core.BlockPos;
import net.minecraft.core.registries.BuiltInRegistries;
import net.minecraft.nbt.*;
import net.minecraft.resources.Identifier;
import net.minecraft.world.level.EmptyBlockGetter;
import net.minecraft.world.level.block.Block;
import net.minecraft.world.level.block.state.BlockState;
import net.minecraft.world.level.block.state.properties.Property;
import net.minecraft.world.level.material.MapColor;

/** Read the actual exported NBT with vanilla, resolve its states and compare
 *  Minecraft's map colours to each exported _map.png. No Go palette is used. */
public class ValidateBlocks {
    @SuppressWarnings({"unchecked", "rawtypes"})
    static BlockState state(CompoundTag entry) {
        Identifier name = Identifier.parse(entry.getString("Name").orElseThrow());
        if (!BuiltInRegistries.BLOCK.containsKey(name)) throw new IllegalArgumentException("Unknown block: " + name);
        Block block = BuiltInRegistries.BLOCK.getValue(name);
        BlockState result = block.defaultBlockState();
        CompoundTag properties = entry.getCompoundOrEmpty("Properties");
        for (String key : properties.keySet()) {
            Property prop = block.getStateDefinition().getProperty(key);
            if (prop == null) throw new IllegalArgumentException(name + " has no property " + key);
            Comparable value = (Comparable)prop.getValue(properties.getString(key).orElseThrow()).orElseThrow();
            result = (BlockState)result.setValue(prop, value);
        }
        return result;
    }

    public static void main(String[] args) throws Exception {
        SharedConstants.tryDetectVersion();
        Bootstrap.bootStrap();
        JsonArray checked = new JsonArray();
        try (var files = Files.list(Path.of(args[0]))) {
            for (Path path : files.filter(p -> p.toString().endsWith(".nbt")).sorted().toList()) {
                CompoundTag root = NbtIo.readCompressed(path, NbtAccounter.unlimitedHeap());
                if (root.getInt("DataVersion").orElseThrow() != 5023) throw new IllegalArgumentException("Wrong DataVersion");
                ListTag palette = root.getListOrEmpty("palette");
                BlockState[] states = new BlockState[palette.size()];
                for (int i = 0; i < states.length; i++) states[i] = state(palette.getCompoundOrEmpty(i));
                int[] top = {-1,-1,-1,-1};
                BlockState[] surface = new BlockState[4];
                for (Tag tag : root.getListOrEmpty("blocks")) {
                    CompoundTag block = (CompoundTag)tag;
                    ListTag pos = block.getListOrEmpty("pos");
                    int y = pos.getInt(1).orElseThrow(), z = pos.getInt(2).orElseThrow();
                    if (y > top[z]) {
                        top[z] = y;
                        surface[z] = states[block.getInt("state").orElseThrow()];
                    }
                }
                BufferedImage image = ImageIO.read(path.resolveSibling(path.getFileName().toString().replace(".nbt", "_map.png")).toFile());
                for (int z = 1; z <= 3; z++) {
                    double delta = (top[z] - top[z-1])*4.0/5 + (((z-1)&1)-.5)*.4;
                    MapColor.Brightness shade = delta>.6 ? MapColor.Brightness.HIGH : delta<-.6 ? MapColor.Brightness.LOW : MapColor.Brightness.NORMAL;
                    MapColor colour = surface[z].getMapColor(EmptyBlockGetter.INSTANCE, BlockPos.ZERO);
                    int actual = colour.calculateARGBColor(shade) & 0xffffff;
                    if (actual != (image.getRGB(0,z-1)&0xffffff)) throw new IllegalArgumentException(path + " map colour mismatch at row " + z);
                }
                JsonObject row = new JsonObject();
                row.addProperty("file", path.getFileName().toString());
                row.addProperty("surface_state", surface[1].toString());
                row.addProperty("all_three_shades_match", true);
                checked.add(row);
            }
        }
        JsonObject report = new JsonObject();
        report.addProperty("minecraft", "26.3");
        report.addProperty("data_version", 5023);
        report.addProperty("variants_checked", checked.size());
        report.add("results", checked);
        Files.writeString(Path.of(args[1]), new GsonBuilder().setPrettyPrinting().create().toJson(report)+"\n");
        System.out.println("Vanilla Java 26.3 verified all 3 shades and NBT states for " + checked.size() + " variants.");
    }
}
